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
	ExtResponseTimeCollector          responsetime.Collector
	Logger                            logging.Logger
	IsDimmingEnabled                  bool
	ResponseTimeCollectorExcludesHTML bool
}

type Server struct {
	FrontendAddr                      string
	BackendAddr                       string
	MaxConns                          int
	ControlLoop                       *ServerControlLoop
	RequestFilter                     *filters.RequestFilter
	PathProbabilities                 *filters.PathProbabilities
	Logger                            logging.Logger
	IsDimmingEnabled                  bool
	ResponseTimeCollectorExcludesHTML bool
	// server and proxy is our reverse proxy implementation which allows
	// requests to be forwarded to the backend host.
	server *fasthttp.Server
	proxy  *fasthttp.HostClient
	// apiMutex ensures that only one caller can access Start and Stop at one
	// time.
	apiMutex  *sync.Mutex
	isStarted bool
	// ExtResponseTimeCollector allows external clients to monitor the response
	// time. The collector is disabled by default.
	ExtResponseTimeCollector          responsetime.Collector
	isExtResponseTimeCollectorStarted bool
}

func NewServer(options *ServerOptions) *Server {
	return &Server{
		FrontendAddr:                      options.FrontendAddr,
		BackendAddr:                       options.BackendAddr,
		MaxConns:                          options.MaxConns,
		ControlLoop:                       options.ControlLoop,
		RequestFilter:                     options.RequestFilter,
		PathProbabilities:                 options.PathProbabilities,
		Logger:                            options.Logger,
		IsDimmingEnabled:                  options.IsDimmingEnabled,
		ResponseTimeCollectorExcludesHTML: options.ResponseTimeCollectorExcludesHTML,
		apiMutex:                          &sync.Mutex{},
		isStarted:                         false,
		ExtResponseTimeCollector:          options.ExtResponseTimeCollector,
		isExtResponseTimeCollectorStarted: false,
	}
}

func (s *Server) Start() error {
	s.apiMutex.Lock()
	defer s.apiMutex.Unlock()

	if s.isStarted {
		return errors.New("server already started")
	}

	s.proxy = &fasthttp.HostClient{Addr: s.BackendAddr, MaxConns: s.MaxConns}
	s.server = &fasthttp.Server{
		Handler:         s.requestHandler(),
		CloseOnShutdown: true,
	}

	s.ControlLoop.mustStart()
	go func() {
		if err := s.server.ListenAndServe(s.FrontendAddr); err != nil {
			log.Fatalf("fasthttp: server error: %v", err)
		}
	}()

	s.isStarted = true
	return nil
}

func (s *Server) Stop() error {
	s.apiMutex.Lock()
	defer s.apiMutex.Unlock()

	if !s.isStarted {
		return errors.New("server not running")
	}

	if err := s.server.Shutdown(); err != nil {
		return fmt.Errorf("unable to stop server; s.shutdown.Shutdown() has err = %w", err)
	}
	s.ControlLoop.mustStop()

	s.isStarted = false
	return nil
}

func (s *Server) ResetControlLoop() error {
	s.apiMutex.Lock()
	defer s.apiMutex.Unlock()

	if !s.isStarted {
		return errors.New("server not running")
	}

	s.ControlLoop.mustStop()
	s.ControlLoop.mustStart()

	return nil
}

func (s *Server) StartExtResponseTimeCollector() {
	s.apiMutex.Lock()
	defer s.apiMutex.Unlock()
	if !s.isExtResponseTimeCollectorStarted {
		s.ExtResponseTimeCollector.Reset()
	}
	s.isExtResponseTimeCollectorStarted = true
}

func (s *Server) StopExtResponseTimeCollector() {
	s.apiMutex.Lock()
	defer s.apiMutex.Unlock()
	if s.isExtResponseTimeCollectorStarted {
		s.ExtResponseTimeCollector.Reset()
	}
	s.isExtResponseTimeCollectorStarted = false
}

func (s *Server) requestHandler() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		req := &ctx.Request
		resp := &ctx.Response

		// If dimming is enabled, enforce dimming on dimmable components by
		// returning a HTTP error page if a probability is met.
		if s.IsDimmingEnabled && s.RequestFilter.Matches(string(ctx.Path()), string(ctx.Method()), string(req.Header.Referer())) {
			if rand.Float64()*100 < s.ControlLoop.readDimmingPercentage() {
				// Dim based on probabilities set with PathProbabilities.
				if rand.Float64() < s.PathProbabilities.Get(string(ctx.Path())) {
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
		if err := s.proxy.Do(req, resp); err != nil {
			ctx.Logger().Printf("fasthttp: error when proxying the request: %v", err)
		}
		duration := time.Now().Sub(startTime)

		// Persist the request time, excluding static .html files if the option
		// for exclusion is enabled.
		if !s.ResponseTimeCollectorExcludesHTML || !strings.Contains(string(ctx.Path()), ".html") {
			s.Logger.LogResponseTime(float64(duration) / float64(time.Second))
			s.ControlLoop.addResponseTime(duration)
			if s.isExtResponseTimeCollectorStarted {
				s.ExtResponseTimeCollector.Add(duration)
			}
		}

		// Remove connection header per RFC2616.
		func(resp *fasthttp.Response) {
			resp.Header.Del("Connection")
		}(resp)
	}
}
