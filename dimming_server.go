package main

import (
	"errors"
	"fmt"
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/responsetimecollector"
	"github.com/valyala/fasthttp"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ServerOptions struct {
	Logger            logging.Logger
	FrontendAddr      string
	BackendAddr       string
	MaxConns          int
	ControlLoop       *ServerControlLoop
	RequestFilter     *filters.RequestFilter
	PathProbabilities *filters.PathProbabilities
	IsDimmingEnabled  bool
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
	dimming struct {
		// IsEnabled states whether dimming should be enabled at
		// production-time.
		IsEnabled bool
		// ControlLoop reads the response time of the server and adjusts the
		// dimming percentage at regular intervals.
		ControlLoop       *ServerControlLoop
		RequestFilter     *filters.RequestFilter
		PathProbabilities *filters.PathProbabilities
	}
	// onlineTraining improves PathProbabilities by randomising the
	// PathProbabilities for a candidate group selected from users being dimmed.
	onlineTraining struct {
		IsEnabled                   bool
		ControlGroupResponseTimes   responsetimecollector.Collector
		CandidateGroupResponseTimes responsetimecollector.Collector
		CandidatePathProbabilities  *filters.PathProbabilities
	}
	// offlineTraining represents the offline training mode. When this mode is
	// enabled, all paths under RequestFilter will be dimmed according to
	// PathProbabilities, regardless of the ControlLoop output.
	offlineTraining struct {
		IsEnabled bool
		// ResponseTimeCollector allows external clients to monitor the response
		// time. The collector is disabled by default.
		ResponseTimeCollector responsetimecollector.Collector
	}
	// isStarted is checked to ensure each Server is only ever started once.
	isStarted bool
	// startStopMux guards external operations which interact with the server.
	startStopMux *sync.Mutex
}

func NewServer(options *ServerOptions) *Server {
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
		dimming: struct {
			IsEnabled         bool
			ControlLoop       *ServerControlLoop
			RequestFilter     *filters.RequestFilter
			PathProbabilities *filters.PathProbabilities
		}{
			ControlLoop:       options.ControlLoop,
			RequestFilter:     options.RequestFilter,
			PathProbabilities: options.PathProbabilities,
			IsEnabled:         options.IsDimmingEnabled,
		},
		onlineTraining: struct {
			IsEnabled                   bool
			ControlGroupResponseTimes   responsetimecollector.Collector
			CandidateGroupResponseTimes responsetimecollector.Collector
			CandidatePathProbabilities  *filters.PathProbabilities
		}{
			IsEnabled:                   false,
			ControlGroupResponseTimes:   responsetimecollector.NewTachymeterCollector(100),
			CandidateGroupResponseTimes: responsetimecollector.NewTachymeterCollector(100),
			CandidatePathProbabilities:  options.PathProbabilities.Copy(),
		},
		offlineTraining: struct {
			IsEnabled             bool
			ResponseTimeCollector responsetimecollector.Collector
		}{
			IsEnabled:             false,
			ResponseTimeCollector: responsetimecollector.NewArrayCollector(),
		},
		isStarted:    false,
		startStopMux: &sync.Mutex{},
	}
}

func (s *Server) ListenAndServe() error {
	s.startStopMux.Lock()
	defer s.startStopMux.Unlock()

	if s.isStarted {
		return errors.New("server already started")
	}

	s.proxying.proxy = &fasthttp.HostClient{Addr: s.proxying.BackendAddr, MaxConns: s.proxying.MaxConns}
	s.proxying.server = &fasthttp.Server{
		Handler:         s.requestHandler(),
		CloseOnShutdown: true,
	}

	if err := s.dimming.ControlLoop.Start(); err != nil {
		return fmt.Errorf("Server.ListenAndServe() got err when calling ControlLoop.ListenAndServe(): %w", err)
	}

	if err := s.proxying.server.ListenAndServe(s.proxying.FrontendAddr); err != nil {
		return fmt.Errorf("Server.ListenAndServe() got fasthttp server error: %w", err)
	}

	s.isStarted = true
	return nil
}

func (s *Server) StartOfflineTrainingMode() error {
	s.startStopMux.Lock()
	defer s.startStopMux.Unlock()

	if !s.isStarted {
		return errors.New("StartOfflineTrainingMode() expected server running; server is not running")
	}

	s.offlineTraining.ResponseTimeCollector.Reset()
	if err := s.dimming.ControlLoop.Reset(); err != nil {
		return fmt.Errorf("Server.StartOfflineTrainingMode() got err when calling ControlLoop.Reset(): %w", err)
	}

	s.offlineTraining.IsEnabled = true
	return nil
}

func (s *Server) StopOfflineTrainingMode() error {
	s.startStopMux.Lock()
	defer s.startStopMux.Unlock()

	if !s.isStarted {
		return errors.New("StopOfflineTrainingMode() expected server running; server is not running")
	}

	s.offlineTraining.ResponseTimeCollector.Reset()
	if err := s.dimming.ControlLoop.Reset(); err != nil {
		return fmt.Errorf("Server.StopOfflineTrainingMode() got err when calling ControlLoop.Reset(): %w", err)
	}

	s.offlineTraining.IsEnabled = false
	return nil
}

func (s *Server) requestHandler() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		req := &ctx.Request
		resp := &ctx.Response

		// If dimming or training mode is enabled, enforce dimming on dimmable
		// components by returning a HTTP error page if a probability is met.
		isDimmingEnabled := s.dimming.IsEnabled || s.offlineTraining.IsEnabled
		isDimmableRequest := s.dimming.RequestFilter.Matches(string(ctx.Path()), string(ctx.Method()), string(req.Header.Referer()))
		if isDimmingEnabled && isDimmableRequest {
			// If offline training is enabled, we always dim. We use a nested
			// conditional to reduce the mutex overhead of reading the dimming
			// percentage.
			shouldDim := s.offlineTraining.IsEnabled ||
				rand.Float64()*100 < s.dimming.ControlLoop.readDimmingPercentage()

			// Ensure dimming is weighted according to path probabilities. Path
			// probabilities are chosen according to whether the request is an
			// online training candidate or not.
			shouldUseOnlineTrainingProbabilities := s.onlineTraining.IsEnabled &&
				hasOnlineTrainingCandidateCookie(req)
			if shouldUseOnlineTrainingProbabilities {
				shouldDim = shouldDim && s.onlineTraining.CandidatePathProbabilities.SampleShouldDim(string(ctx.Path()))
			} else {
				shouldDim = shouldDim && s.dimming.PathProbabilities.SampleShouldDim(string(ctx.Path()))
			}

			if shouldDim {
				ctx.Error("dimming", http.StatusTooManyRequests)
				return
			}
		}

		// Remove connection header per RFC2616.
		func(req *fasthttp.Request) {
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

			if s.offlineTraining.IsEnabled {
				s.offlineTraining.ResponseTimeCollector.Add(duration)
			}

			if s.onlineTraining.IsEnabled && hasOnlineTrainingCookie(req) {
				if hasOnlineTrainingCandidateCookie(req) {
					s.onlineTraining.CandidateGroupResponseTimes.Add(duration)
				} else {
					s.onlineTraining.ControlGroupResponseTimes.Add(duration)
				}
			}
		}

		// Remove connection header per RFC2616.
		func(resp *fasthttp.Response) {
			resp.Header.Del("Connection")

			// Only set an online training cookie for .html pages. If this
			// restriction did not exist, a cookie could be sampled several
			// times for each of the API requests associated with a single
			// page, despite the user only visiting one page.
			if s.onlineTraining.IsEnabled && !hasOnlineTrainingCookie(req) {
				resp.Header.Cookie(sampleOnlineTrainingCookie())
			}
		}(resp)
	}
}
