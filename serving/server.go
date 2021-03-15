package serving

import (
	"errors"
	"fmt"
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/monitoring/responsetime"
	"github.com/valyala/fasthttp"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type ServerOptions struct {
	FrontendAddr      string
	BackendAddr       string
	MaxConns          int
	ControlLoop       *ServerControlLoop
	RequestFilter     *filters.RequestFilter
	PathProbabilities *filters.PathProbabilities
	Logger            logging.Logger
	IsDimmingEnabled  bool
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
		// IsEnabled states whether dimming should be enabled at
		// production-time.
		IsEnabled bool
		// ControlLoop reads the response time of the server and adjusts the
		// dimming percentage at regular intervals.
		ControlLoop       *ServerControlLoop
		RequestFilter     *filters.RequestFilter
		PathProbabilities *filters.PathProbabilities
	}
	// offlineTraining represents the offline training mode. When this mode is
	// enabled, all paths under RequestFilter will be dimmed according to
	// PathProbabilities, regardless of the ControlLoop output.
	offlineTraining struct {
		IsEnabled bool
		// ResponseTimeCollector allows external clients to monitor the response
		// time. The collector is disabled by default.
		ResponseTimeCollector responsetime.Collector
	}
	// isStarted is checked to ensure each Server is only ever started once.
	isStarted bool
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
		offlineTraining: struct {
			IsEnabled             bool
			ResponseTimeCollector responsetime.Collector
		}{
			IsEnabled:             false,
			ResponseTimeCollector: responsetime.NewArrayCollector(),
		},
		isStarted: false,
	}
}

func (s *Server) Start() error {
	if s.isStarted {
		return errors.New("server already started")
	}

	s.proxying.proxy = &fasthttp.HostClient{Addr: s.proxying.BackendAddr, MaxConns: s.proxying.MaxConns}
	s.proxying.server = &fasthttp.Server{
		Handler:         s.requestHandler(),
		CloseOnShutdown: true,
	}

	if err := s.dimming.ControlLoop.Start(); err != nil {
		return fmt.Errorf("Server.Start() got err when calling ControlLoop.Start(): %w", err)
	}
	go func() {
		if err := s.proxying.server.ListenAndServe(s.proxying.FrontendAddr); err != nil {
			log.Fatalf("fasthttp: server error: %v", err)
		}
	}()

	s.isStarted = true
	return nil
}

func (s *Server) StartOfflineTrainingMode() error {
	if !s.isStarted {
		return errors.New("StartOfflineTrainingMode() expected server running; server is not running")
	}

	s.offlineTraining.ResponseTimeCollector.Reset()
	if err := s.dimming.ControlLoop.Stop(); err != nil {
		return fmt.Errorf("Server.StartOfflineTrainingMode() got err when calling ControlLoop.Stop(): %w", err)
	}
	if err := s.dimming.ControlLoop.Start(); err != nil {
		return fmt.Errorf("Server.StartOfflineTrainingMode() got err when calling ControlLoop.Start(): %w", err)
	}

	s.offlineTraining.IsEnabled = true
	return nil
}

func (s *Server) StopOfflineTrainingMode() error {
	if !s.isStarted {
		return errors.New("StopOfflineTrainingMode() expected server running; server is not running")
	}

	s.offlineTraining.ResponseTimeCollector.Reset()
	if err := s.dimming.ControlLoop.Stop(); err != nil {
		return fmt.Errorf("Server.StopOfflineTrainingMode() got err when calling ControlLoop.Stop(): %w", err)
	}
	if err := s.dimming.ControlLoop.Start(); err != nil {
		return fmt.Errorf("Server.StopOfflineTrainingMode() got err when calling ControlLoop.Start(): %w", err)
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
		if (s.dimming.IsEnabled || s.offlineTraining.IsEnabled) &&
			s.dimming.RequestFilter.Matches(string(ctx.Path()), string(ctx.Method()), string(req.Header.Referer())) {
			// If offline training is enabled, we always dim.
			if s.offlineTraining.IsEnabled || rand.Float64()*100 < s.dimming.ControlLoop.readDimmingPercentage() {
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

		// Send the request time to the dimming control loop regardless of
		// whether dimming is actually enabled, so monitoring tools can capture
		// what the dimmer would do if enabled. Static .html files are excluded
		// from the control loop as these cache-able files cause bias.
		if !strings.Contains(string(ctx.Path()), ".html") {
			s.dimming.ControlLoop.addResponseTime(duration)
			if s.offlineTraining.IsEnabled {
				s.offlineTraining.ResponseTimeCollector.Add(duration)
			}
		}

		// Remove connection header per RFC2616.
		func(resp *fasthttp.Response) {
			resp.Header.Del("Connection")
		}(resp)
	}
}
