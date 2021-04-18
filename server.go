package main

import (
	"errors"
	"fmt"
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/offlinetraining"
	"github.com/kcz17/dimmer/onlinetraining"
	"github.com/kcz17/dimmer/profiling"
	"github.com/valyala/fasthttp"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type DimmingMode int

const (
	Disabled DimmingMode = iota
	OfflineTraining
	Dimming
	DimmingWithProfiling
	DimmingWithOnlineTraining
)

type ServerOptions struct {
	Logger                 logging.Logger
	FrontendAddr           string
	BackendAddr            string
	MaxConns               int
	ControlLoop            *ServerControlLoop
	RequestFilter          *filters.RequestFilter
	PathProbabilities      *filters.PathProbabilities
	OnlineTrainingService  *onlinetraining.OnlineTraining
	OfflineTrainingService *offlinetraining.OfflineTraining
	IsProfilingEnabled     bool
	ProfilingService       *profiling.Profiler
	ProfilingSessionCookie string
	IsDimmingEnabled       bool
}

// Server is a dimming-enhanced server. Dimming is actuated using a control
// loop in requestHandler(), which uses conditionally performs dimming on
// requests specified by the RequestFilter and path-dependent probabilities
// specified by PathProbabilities.
type Server struct {
	logger   logging.Logger
	proxying struct {
		FrontendAddr string
		BackendAddr  string
		MaxConns     int
		// server and proxy implement our reverse proxy, allowing requests
		// to be forwarded to the backend host.
		server *fasthttp.Server
		proxy  *fasthttp.HostClient
	}
	dimmingMode        DimmingMode
	defaultDimmingMode DimmingMode
	dimming            struct {
		// ControlLoop reads the response time of the server and adjusts the
		// dimming percentage at regular intervals.
		ControlLoop       *ServerControlLoop
		RequestFilter     *filters.RequestFilter
		PathProbabilities *filters.PathProbabilities
	}
	// onlineTraining improves PathProbabilities by randomising the
	// PathProbabilities for a candidate group selected from users being dimmed.
	onlineTraining *onlinetraining.OnlineTraining
	// offlineTraining represents the offline training mode. When this mode is
	// enabled, all paths under RequestFilter will be dimmed according to
	// PathProbabilities, regardless of the ControlLoop output.
	offlineTraining    *offlinetraining.OfflineTraining
	isProfilingEnabled bool
	// profiling actuates dimming based on user priority, ensuring users have
	// experience the website with dimming consistent to their profiled priority.
	profiling              *profiling.Profiler
	profilingSessionCookie string
	// isStarted is checked to ensure each Server is only ever started once.
	isStarted bool
	// externalOperationsLock guards external operations which interact with the server.
	externalOperationsLock *sync.Mutex
}

func NewServer(options *ServerOptions) *Server {
	defaultMode := Disabled
	if options.IsDimmingEnabled {
		defaultMode = Dimming
	}

	return &Server{
		logger: options.Logger,
		proxying: struct {
			FrontendAddr string
			BackendAddr  string
			MaxConns     int
			server       *fasthttp.Server
			proxy        *fasthttp.HostClient
		}{
			FrontendAddr: options.FrontendAddr,
			BackendAddr:  options.BackendAddr,
			MaxConns:     options.MaxConns,
			server:       nil,
			proxy:        nil,
		},
		dimmingMode:        defaultMode,
		defaultDimmingMode: defaultMode,
		dimming: struct {
			ControlLoop       *ServerControlLoop
			RequestFilter     *filters.RequestFilter
			PathProbabilities *filters.PathProbabilities
		}{
			ControlLoop:       options.ControlLoop,
			RequestFilter:     options.RequestFilter,
			PathProbabilities: options.PathProbabilities,
		},
		onlineTraining:         options.OnlineTrainingService,
		offlineTraining:        options.OfflineTrainingService,
		profiling:              options.ProfilingService,
		profilingSessionCookie: options.ProfilingSessionCookie,
		isProfilingEnabled:     options.IsProfilingEnabled,
		isStarted:              false,
		externalOperationsLock: &sync.Mutex{},
	}
}

func (s *Server) ListenAndServe() error {
	s.externalOperationsLock.Lock()

	if s.isStarted {
		return errors.New("server already started")
	}

	s.proxying.proxy = &fasthttp.HostClient{Addr: s.proxying.BackendAddr, MaxConns: s.proxying.MaxConns}
	s.proxying.server = &fasthttp.Server{
		Handler:         s.requestHandler(),
		CloseOnShutdown: true,
	}
	s.isStarted = true

	if err := s.dimming.ControlLoop.Start(); err != nil {
		return fmt.Errorf("Server.ListenAndServe() got err when calling ControlLoop.ListenAndServe(): %w", err)
	}

	s.externalOperationsLock.Unlock()

	if err := s.proxying.server.ListenAndServe(s.proxying.FrontendAddr); err != nil {
		return fmt.Errorf("Server.ListenAndServe() got fasthttp server error: %w", err)
	}

	return nil
}

func (s *Server) UpdatePathProbabilities(rules []filters.PathProbabilityRule) error {
	// Path probabilities affect both dimming and online training, hence both
	// must be accurately set.
	if err := s.dimming.PathProbabilities.SetAll(rules); err != nil {
		return fmt.Errorf("expected PathProbabilities.SetAll(probabilities = %+v) to return err != nil; got err = %w", rules, err)
	}

	var paths []string
	for _, rule := range rules {
		paths = append(paths, rule.Path)
	}
	s.onlineTraining.SetPaths(paths)

	return nil
}

func (s *Server) SetDimmingMode(newMode DimmingMode) error {
	s.externalOperationsLock.Lock()
	defer s.externalOperationsLock.Unlock()

	if !s.isStarted {
		return errors.New("SetDimmingMode() expected server running; server is not running")
	}

	if s.dimmingMode == DimmingWithOnlineTraining {
		if err := s.onlineTraining.StopLoop(); err != nil {
			return fmt.Errorf("expected onlineTraining.StopLoop() returns nil err; got err = %w", err)
		}
	}

	s.offlineTraining.ResetCollector()
	if err := s.dimming.ControlLoop.Reset(); err != nil {
		return fmt.Errorf("expected ControlLoop.Reset() returns nil err; got err = %w", err)
	}

	if newMode == DimmingWithOnlineTraining {
		if err := s.onlineTraining.StartLoop(); err != nil {
			return fmt.Errorf("expected onlineTraining.StartLoop() returns nil err; got err = %w", err)
		}
	}

	s.dimmingMode = newMode
	return nil
}

func (s *Server) requestHandler() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		req := &ctx.Request
		resp := &ctx.Response

		// Remove connection header from response per RFC2616.
		resp.Header.Del("Connection")

		// If dimming or training mode is enabled, enforce dimming on dimmable
		// components by returning a HTTP error page if a probability is met.
		isDimmingEnabled := s.dimmingMode != Disabled
		isDimmableRequest := s.dimming.RequestFilter.Matches(string(ctx.Path()), string(ctx.Method()), string(req.Header.Referer()))
		if isDimmingEnabled && isDimmableRequest {
			// If offline training is enabled, we always dim. shouldDim is
			// nested inside an if statement instead of being top-level to
			// eliminate the mutex overhead of reading the dimming percentage if
			// the request is not dimmable.
			shouldDim := s.dimmingMode == OfflineTraining ||
				rand.Float64()*100 < s.dimming.ControlLoop.readDimmingPercentage()

			// Profiled sessions which are dimmed as a result of their priority
			// will have all optional components uniformly dimmed irrespective
			// of path probabilities.
			skipPathProbabilities := false

			// Profiling should only occur when the session cookie is set.
			if s.isProfilingEnabled && s.dimmingMode == DimmingWithProfiling &&
				len(req.Header.Cookie(s.profilingSessionCookie)) != 0 {
				if profiling.HasDimmingDecisionCookie(req) {
					// If the session is dimmed as a result of its priority, we
					// override the dimmer to always dim optional components.
					skipPathProbabilities = true
					shouldDim = profiling.ReadDimmingDecisionCookie(req)
				} else if profiling.RequestHasPriorityCookie(req) {
					// Sample a long-term dimming decision as the session has a
					// priority profiled but its dimming decision has not been
					// made. We use the existing shouldDim variable for this
					// decision as it is declared above by sampling against the
					// current PID output.
					dimmingDecision := shouldDim && profiling.SampleDimmingForPriorityCookie(req)

					// Persist the dimming decision. We do not actuate dimming
					// for the current request even if the dimming decision is
					// true, as the response headers would otherwise be reset by
					// the ctx.Error call below.
					resp.Header.SetCookie(profiling.CookieForDimmingDecision(dimmingDecision))

					// Actuate the dimming decision for the current request.
					skipPathProbabilities = dimmingDecision
					shouldDim = shouldDim || dimmingDecision
				}
			}

			if !skipPathProbabilities {
				// Ensure dimming is weighted according to path probabilities. Path
				// probabilities are chosen according to whether the request is an
				// online training candidate or not.
				shouldUseOnlineTrainingCandidateGroupProbabilities :=
					s.dimmingMode == DimmingWithOnlineTraining &&
						onlinetraining.RequestHasCandidateCookie(req)

				if shouldUseOnlineTrainingCandidateGroupProbabilities {
					shouldDim = shouldDim && s.onlineTraining.SampleCandidateGroupShouldDim(string(ctx.Path()))
				} else {
					shouldDim = shouldDim && s.dimming.PathProbabilities.SampleShouldDim(string(ctx.Path()))
				}
			}

			if shouldDim {
				ctx.SetStatusCode(http.StatusTooManyRequests)
				ctx.SetBodyString("Dimming!")
				return
			}
		}

		func(req *fasthttp.Request) {
			// Remove connection header per RFC2616.
			req.Header.Del("Connection")
		}(req)

		// Proxy the request, capturing the request time.
		startTime := time.Now()
		if err := s.proxying.proxy.Do(req, resp); err != nil {
			ctx.Logger().Printf("fasthttp: error when proxying the request: %v", err)
		}
		duration := time.Now().Sub(startTime)

		// Send the request time to the dimming control loop regardless of
		// whether dimming is actually enabled, so monitoring tools can capture
		// what the dimmer would do if enabled. Static .html files are excluded
		// from the control loop as these cache-able files cause bias.
		if !strings.Contains(string(ctx.Path()), ".html") {
			s.dimming.ControlLoop.addResponseTime(duration)

			if s.dimmingMode == OfflineTraining {
				s.offlineTraining.AddResponseTime(duration)
			}

			if s.dimmingMode == DimmingWithOnlineTraining &&
				onlinetraining.RequestHasCookie(req) {
				if onlinetraining.RequestHasCandidateCookie(req) {
					s.onlineTraining.AddCandidateResponseTime(duration)
				} else {
					s.onlineTraining.AddControlResponseTime(duration)
				}
			}
		}

		// If profiling is enabled, save the request for further profiling and
		// set appropriate profiling cookies if none exist.
		if s.isProfilingEnabled && s.dimmingMode == DimmingWithProfiling &&
			len(req.Header.Cookie(s.profilingSessionCookie)) != 0 {
			s.profiling.Requests.Write(string(req.Header.Cookie(s.profilingSessionCookie)), string(ctx.Method()), string(ctx.Path()))

			// Fetch the session's priority if it does not have a priority set.
			if !profiling.RequestHasUnknownCookie(req) &&
				strings.Contains(string(ctx.Path()), ".html") {
				sessionID := string(req.Header.Cookie(s.profilingSessionCookie))
				priority, err := s.profiling.Priorities.Fetch(sessionID)
				if err != nil {
					log.Printf("could not fetch priority for sessionID = %s due to err %s", sessionID, err)
				} else {
					resp.Header.SetCookie(profiling.CookieForPriority(priority))

					// Profiler implementations may require a push to an external
					// service profile unknown sessions.
					if priority == profiling.Unknown {
						s.profiling.Priorities.Profile(sessionID)
					}
				}
			}
		}

		// Only set an online training cookie for .html pages. If this
		// restriction did not exist, a cookie could be sampled several
		// times for each of the API requests associated with a single
		// page, despite the user only visiting one page.
		if s.dimmingMode == DimmingWithOnlineTraining &&
			strings.Contains(string(ctx.Path()), ".html") &&
			!onlinetraining.RequestHasCookie(req) {
			resp.Header.SetCookie(onlinetraining.SampleCookie())
		}
	}
}
