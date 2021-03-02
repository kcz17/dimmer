package main

import (
	"fmt"
	"github.com/kcz17/dimmer/controller"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/monitoring/response_time"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
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
	ResponseTimeCollectorRequestsWindow int `env:"NUM_REQUESTS_WINDOW"`

	// ResponseTimeCollectorExcludesHTML excludes response time capturing for
	// .html files. Used to ensure that response time calculations are not
	// biased by the low response times of static files.
	ResponseTimeCollectorExcludesHTML bool `env:"LOGGER_EXCLUDE_HTML" env-default:"false"`
}

func main() {
	var config Config
	err := cleanenv.ReadEnv(&config)
	if err != nil {
		log.Fatalf("expected err == nil in envconfig.Process(); got err = %v", err)
	} else if config.ControllerPercentile != "p50" && config.ControllerPercentile != "p75" && config.ControllerPercentile != "p95" {
		log.Fatalf("expected enviornment variable CONTROLLER_PERCENTILE to be one of {p50|p75|p95}; got %s", config.ControllerPercentile)
	}

	requestFilter := initRequestFilter()
	logger := initLogger(&config)
	pid, err := initPIDController(&config)
	if err != nil {
		log.Fatalf("expected controller.NewPIDController() returns nil err; got err = %v", err)
	}
	responseTimeCollector := response_time.NewTachymeterResponseTimeCollector(config.ResponseTimeCollectorRequestsWindow)

	controllerOutputMux := &sync.RWMutex{}
	controllerOutput := 0.0
	go controlLoop(responseTimeCollector, pid, logger, config.ControllerPercentile, &controllerOutput, controllerOutputMux)

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
			controllerOutputMux.RLock()
			dimmingPercentage := controllerOutput
			controllerOutputMux.RUnlock()

			if rand.Float64()*100 < dimmingPercentage {
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

func initPIDController(config *Config) (*controller.PIDController, error) {
	return controller.NewPIDController(
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
		//during "full" dimming even if requests are only made to dimmed components.
		99,
		config.ControllerSamplePeriod,
	)
}

func controlLoop(
	responseTimes response_time.ResponseTimeCollector,
	pid *controller.PIDController,
	logger logging.Logger,
	dimmingPercentile string,
	dimmingPercentage *float64,
	dimmingPercentageMux *sync.RWMutex,
) {
	for range time.Tick(time.Second * 1) {
		aggregation := responseTimes.Aggregate()

		// PID controller and logger operate with seconds.
		p50 := float64(aggregation.P50) / float64(time.Second)
		p75 := float64(aggregation.P75) / float64(time.Second)
		p95 := float64(aggregation.P95) / float64(time.Second)
		logger.LogAggregateResponseTimes(p50, p75, p95)

		var pidOutput float64
		if dimmingPercentile == "p50" {
			pidOutput = pid.Output(p50)
		} else if dimmingPercentile == "p75" {
			pidOutput = pid.Output(p75)
		} else if dimmingPercentile == "p95" {
			pidOutput = pid.Output(p95)
		} else {
			log.Fatalf("controlLoop() expected dimmingPercentile to be one of {50|75|95}; got %s", dimmingPercentile)
		}
		logger.LogDimmerOutput(pidOutput)
		logger.LogPIDControllerState(pid.DebugP, pid.DebugI, pid.DebugD, pid.DebugErr)

		// Apply the PID output.
		dimmingPercentageMux.Lock()
		*dimmingPercentage = pidOutput
		dimmingPercentageMux.Unlock()
	}
}
