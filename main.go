package main

import (
	"fmt"
	"github.com/kcz17/dimmer/controller"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/monitoring/responsetime"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/valyala/fasthttp"
)

type Config struct {
	///////////////////////////////////////////////////////////////////////////
	// Proxying and serving.
	///////////////////////////////////////////////////////////////////////////

	FrontEndPort string `env:"FE_PORT"`
	BackEndHost  string `env:"BE_HOST" env-default:"localhost"`
	BackEndPort  string `env:"BE_PORT"`

	///////////////////////////////////////////////////////////////////////////
	// Logging.
	///////////////////////////////////////////////////////////////////////////

	LoggerDriver        string `env:"LOGGER_DRIVER" env-default:"noop"`
	LoggerInfluxDBHost  string `env:"LOGGER_INFLUXDB_HOST"`
	LoggerInfluxDBToken string `env:"LOGGER_INFLUXDB_TOKEN"`

	///////////////////////////////////////////////////////////////////////////
	// General dimming.
	///////////////////////////////////////////////////////////////////////////

	IsDimmerEnabled        bool    `env:"DIMMER_ENABLED" env-default:"true"`
	ControllerSamplePeriod float64 `env:"CONTROLLER_SAMPLE_PERIOD"`
	ControllerPercentile   string  `env:"CONTROLLER_PERCENTILE" env-default:"p95"`
	ControllerSetpoint     float64 `env:"CONTROLLER_SETPOINT"`
	ControllerKp           float64 `env:"CONTROLLER_KP"`
	ControllerKi           float64 `env:"CONTROLLER_KI"`
	ControllerKd           float64 `env:"CONTROLLER_KD"`

	///////////////////////////////////////////////////////////////////////////
	// Response time data collection.
	///////////////////////////////////////////////////////////////////////////

	// ResponseTimeCollectorRequestsWindow defines the number of requests used
	// to aggregate response time metrics. It should be smaller than or equal to
	// the number of expected requests received during the sample period.
	ResponseTimeCollectorRequestsWindow int `env:"RESPONSE_TIME_COLLECTOR_NUM_REQUESTS_WINDOW"`

	// ResponseTimeCollectorExcludesHTML excludes response time capturing for
	// .html files. Used to ensure that response time calculations are not
	// biased by the low response times of static files.
	ResponseTimeCollectorExcludesHTML bool `env:"RESPONSE_TIME_COLLECTOR_EXCLUDES_HTML" env-default:"false"`
}

func main() {
	var config Config
	err := cleanenv.ReadEnv(&config)
	if err != nil {
		log.Fatalf("expected err == nil in envconfig.Process(); got err = %v", err)
	}

	logger := initLogger(&config)
	requestFilter := initRequestFilter()
	pathProbabilities := initPathProbabilities()
	responseTimeCollector := responsetime.NewTachymeterCollector(config.ResponseTimeCollectorRequestsWindow)
	controlLoop := initControlLoop(&config, initPIDController(&config), responseTimeCollector, logger)

	proxy := &fasthttp.HostClient{
		Addr:     config.BackEndHost + ":" + config.BackEndPort,
		MaxConns: 2048,
	}

	if err := fasthttp.ListenAndServe(fmt.Sprintf(":%v", config.FrontEndPort), func(ctx *fasthttp.RequestCtx) {
		req := &ctx.Request
		resp := &ctx.Response

		// If dimming is enabled, enforce dimming on dimmable components by
		// returning a HTTP error page if a probability is met.
		if config.IsDimmerEnabled && requestFilter.Matches(string(ctx.Path()), string(ctx.Method()), string(req.Header.Referer())) {
			if rand.Float64()*100 < controlLoop.ReadDimmingPercentage() {
				// Dim based on probabilities set with PathProbabilities.
				if rand.Float64() < pathProbabilities.Get(string(ctx.Path())) {
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
		if err := proxy.Do(req, resp); err != nil {
			ctx.Logger().Printf("fasthttp: error when proxying the request: %v", err)
		}
		duration := time.Now().Sub(startTime)

		// Persist the request time, excluding static .html files if the option
		// for exclusion is enabled.
		if !config.ResponseTimeCollectorExcludesHTML || !strings.Contains(string(ctx.Path()), ".html") {
			logger.LogResponseTime(float64(duration) / float64(time.Second))
			responseTimeCollector.Add(duration)
		}

		// Remove connection header per RFC2616.
		func(resp *fasthttp.Response) {
			resp.Header.Del("Connection")
		}(resp)
	}); err != nil {
		log.Fatalf("fasthttp: server error: %v", err)
	}
}

func initLogger(config *Config) logging.Logger {
	var logger logging.Logger
	if config.LoggerDriver == "noop" {
		logger = logging.NewNoopLogger()
	} else if config.LoggerDriver == "stdout" {
		logger = logging.NewStdoutLogger()
	} else if config.LoggerDriver == "influxdb" {
		logger = logging.NewInfluxDBLogger(config.LoggerInfluxDBHost, config.LoggerInfluxDBToken)
	} else {
		log.Fatalf("expected env var LOGGER_DRIVER one of {noop, stdout, influxdb}; got %s", config.LoggerDriver)
	}
	return logger
}

func initRequestFilter() *RequestFilter {
	filter := NewRequestFilter()
	filter.AddPathForAllMethods("recommender")
	filter.AddPathForAllMethods("news.html")
	filter.AddPathForAllMethods("news")
	filter.AddPath("cart", "GET")
	if err := filter.AddRefererExclusion("cart", "GET", "basket.html"); err != nil {
		panic(fmt.Sprintf("expected filter.AddRefererExclusion() returns nil err; got err = %v", err))
	}
	return filter
}

func initPathProbabilities() *PathProbabilities {
	// Set the defaultValue to 1 so we allow dimming by default for paths which
	// are not in the probabilities list.
	p, err := NewPathProbabilities(1)
	if err != nil {
		panic(fmt.Sprintf("expected initPathProbabilities() returns nil err; got err = %v", err))
	}
	return p
}

func initPIDController(config *Config) *controller.PIDController {
	c, err := controller.NewPIDController(
		controller.NewRealtimeClock(),
		config.ControllerSetpoint,
		config.ControllerKp,
		config.ControllerKi,
		config.ControllerKd,
		// isReversed is true as we want a positive error (i.e., actual response
		// time below desired setpoint) to reduce the controller output.
		true,
		// minOutput is 0 as we do not want any dimming when the response time
		// does not violate the desired setpoint.
		0,
		// maxOutput is 99 instead of 100 to ensure response times are collected
		// during "full" dimming even if requests are only made to dimmed
		// components.
		99,
		config.ControllerSamplePeriod,
	)
	if err != nil {
		log.Fatalf("expected controller.NewPIDController() returns nil err; got err = %v", err)
	}

	return c
}

func initControlLoop(
	config *Config,
	pid *controller.PIDController,
	responseTimeCollector responsetime.Collector,
	logger logging.Logger,
) *DimmerControlLoop {
	if config.ControllerPercentile != "p50" && config.ControllerPercentile != "p75" && config.ControllerPercentile != "p95" {
		log.Fatalf("expected environment variable CONTROLLER_PERCENTILE to be one of {p50|p75|p95}; got %s", config.ControllerPercentile)
	}

	c, err := StartNewDimmerControlLoop(pid, responseTimeCollector, config.ControllerPercentile, logger)
	if err != nil {
		log.Fatalf("expected StartNewDimmerControlLoop() returns nil err; got err = %v", err)
	}

	return c
}
