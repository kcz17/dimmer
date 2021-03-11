package serving

import (
	"errors"
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/monitoring/responsetime"
	"github.com/valyala/fasthttp"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ServerOptions struct {
	FrontendAddr                      string
	BackendAddr                       string
	MaxConns                          int
	ControlLoop                       *ServerControlLoop
	RequestFilter                     *filters.RequestFilter
	PathProbabilities                 *filters.PathProbabilities
	Logger                            logging.Logger
	IsDimmingEnabled                  bool
	ResponseTimeCollectorExcludesHTML bool
}

// Server is a dimming-enhanced server. Dimming is actuated using a control
// loop in s.requestHandler, which uses allows dimming on select paths, with
// each path associated with a probability it will be dimmed.
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
		// IsDimmingEnabled states whether dimming should be enabled at
		// production-time.
		IsDimmingEnabled bool
		// ControlLoop reads the response time of the server and adjusts the
		// dimming percentage at regular intervals.
		ControlLoop                       *ServerControlLoop
		RequestFilter                     *filters.RequestFilter
		PathProbabilities                 *filters.PathProbabilities
		ResponseTimeCollectorExcludesHTML bool
	}
	// offlineTraining represents the offline training mode. When this mode is
	// enabled, all paths under RequestFilter will be dimmed according to
	// PathProbabilities, regardless of the ControlLoop output.
	offlineTraining struct {
		// ExtResponseTimeCollector allows external clients to monitor the response
		// time. The collector is disabled by default.
		ExtResponseTimeCollector          responsetime.Collector
		isExtResponseTimeCollectorStarted bool
	}
	// isStarted is checked to ensure each Server is only ever started once.
	isStarted bool
	// mux synchronises server operations which can be called concurrently,
	// e.g., via the offline training API.
	mux *sync.Mutex
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
			IsDimmingEnabled                  bool
			ControlLoop                       *ServerControlLoop
			RequestFilter                     *filters.RequestFilter
			PathProbabilities                 *filters.PathProbabilities
			ResponseTimeCollectorExcludesHTML bool
		}{
			ControlLoop:                       options.ControlLoop,
			RequestFilter:                     options.RequestFilter,
			PathProbabilities:                 options.PathProbabilities,
			IsDimmingEnabled:                  options.IsDimmingEnabled,
			ResponseTimeCollectorExcludesHTML: options.ResponseTimeCollectorExcludesHTML,
		},
		offlineTraining: struct {
			ExtResponseTimeCollector          responsetime.Collector
			isExtResponseTimeCollectorStarted bool
		}{
			ExtResponseTimeCollector:          responsetime.NewArrayCollector(),
			isExtResponseTimeCollectorStarted: false,
		},
		isStarted: false,
		mux:       &sync.Mutex{},
	}
}

func (s *Server) Start() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if s.isStarted {
		return errors.New("server already started")
	}

	s.proxying.proxy = &fasthttp.HostClient{Addr: s.proxying.BackendAddr, MaxConns: s.proxying.MaxConns}
	s.proxying.server = &fasthttp.Server{
		Handler:         s.requestHandler(),
		CloseOnShutdown: true,
	}

	s.dimming.ControlLoop.mustStart()
	go func() {
		if err := s.proxying.server.ListenAndServe(s.proxying.FrontendAddr); err != nil {
			log.Fatalf("fasthttp: server error: %v", err)
		}
	}()

	s.isStarted = true
	return nil
}

func (s *Server) ResetControlLoop() error {
	s.mux.Lock()
	defer s.mux.Unlock()

	if !s.isStarted {
		return errors.New("server not running")
	}

	s.dimming.ControlLoop.mustStop()
	s.dimming.ControlLoop.mustStart()

	return nil
}

func (s *Server) StartExtResponseTimeCollector() {
	s.mux.Lock()
	defer s.mux.Unlock()
	if !s.offlineTraining.isExtResponseTimeCollectorStarted {
		s.offlineTraining.ExtResponseTimeCollector.Reset()
	}
	s.offlineTraining.isExtResponseTimeCollectorStarted = true
}

func (s *Server) StopExtResponseTimeCollector() {
	s.mux.Lock()
	defer s.mux.Unlock()
	if s.offlineTraining.isExtResponseTimeCollectorStarted {
		s.offlineTraining.ExtResponseTimeCollector.Reset()
	}
	s.offlineTraining.isExtResponseTimeCollectorStarted = false
}

func (s *Server) requestHandler() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		req := &ctx.Request
		resp := &ctx.Response

		// If dimming is enabled, enforce dimming on dimmable components by
		// returning a HTTP error page if a probability is met.
		if s.dimming.IsDimmingEnabled && s.dimming.RequestFilter.Matches(string(ctx.Path()), string(ctx.Method()), string(req.Header.Referer())) {
			if rand.Float64()*100 < s.dimming.ControlLoop.readDimmingPercentage() {
				// Dim based on probabilities set with PathProbabilities.
				if rand.Float64() < s.dimming.PathProbabilities.Get(string(ctx.Path())) {
					ctx.Error("dimming", http.StatusTooManyRequests)
					return
				}
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

		// Persist the request time, excluding static .html files if the option
		// for exclusion is enabled.
		if !s.dimming.ResponseTimeCollectorExcludesHTML || !strings.Contains(string(ctx.Path()), ".html") {
			s.logger.LogResponseTime(float64(duration) / float64(time.Second))
			s.dimming.ControlLoop.addResponseTime(duration)
			if s.offlineTraining.isExtResponseTimeCollectorStarted {
				s.offlineTraining.ExtResponseTimeCollector.Add(duration)
			}
		}

		// Remove connection header per RFC2616.
		func(resp *fasthttp.Response) {
			resp.Header.Del("Connection")
		}(resp)
	}
}
