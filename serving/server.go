package serving

import (
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/logging"
	"github.com/valyala/fasthttp"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

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
	server    *fasthttp.Server
	proxy     *fasthttp.HostClient
	isStarted bool
}

func (s *Server) Start() {
	if s.isStarted {
		panic("server already started")
	}

	s.proxy = &fasthttp.HostClient{Addr: s.BackendAddr, MaxConns: s.MaxConns}
	s.server = &fasthttp.Server{
		Handler:         s.requestHandler(),
		CloseOnShutdown: true,
	}

	s.ControlLoop.start()
	go func() {
		if err := s.server.ListenAndServe(s.FrontendAddr); err != nil {
			log.Fatalf("fasthttp: server error: %v", err)
		}
	}()

	s.isStarted = true
}

func (s *Server) Restart() {
	if !s.isStarted {
		panic("server not yet started")
	}

	if err := s.server.Shutdown(); err != nil {
		panic(err)
	}

	s.proxy = &fasthttp.HostClient{Addr: s.BackendAddr, MaxConns: s.MaxConns}
	s.server = &fasthttp.Server{
		Handler:         s.requestHandler(),
		CloseOnShutdown: true,
	}

	s.ControlLoop.restart()
	go func() {
		if err := s.server.ListenAndServe(s.FrontendAddr); err != nil {
			log.Fatalf("fasthttp: server error: %v", err)
		}
	}()
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
		}

		// Remove connection header per RFC2616.
		func(resp *fasthttp.Response) {
			resp.Header.Del("Connection")
		}(resp)
	}
}
