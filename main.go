package main

import (
	"fmt"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/monitoring/responsetime"
	"github.com/kcz17/dimmer/pidcontroller"
	serving "github.com/kcz17/dimmer/serving"
	"log"
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
	// Dimming controller.
	///////////////////////////////////////////////////////////////////////////

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

	controlLoop := initControlLoop(
		&config,
		initPIDController(&config),
		responsetime.NewTachymeterCollector(config.ResponseTimeCollectorRequestsWindow),
		logger,
	)

	// Filters used to selectively dim routes.
	requestFilter := initRequestFilter()
	pathProbabilities := initPathProbabilities()

	// If the admin API puts the server into training mode, the server will use
	// the array collector.
	extResponseTimeCollector := responsetime.NewArrayCollector()

	// Serve the reverse proxy with dimming control loop.
	server := serving.NewServer(&serving.ServerOptions{
		FrontendAddr:                      fmt.Sprintf(":%v", config.FrontEndPort),
		BackendAddr:                       config.BackEndHost + ":" + config.BackEndPort,
		MaxConns:                          2048,
		ControlLoop:                       controlLoop,
		RequestFilter:                     requestFilter,
		PathProbabilities:                 pathProbabilities,
		ExtResponseTimeCollector:          extResponseTimeCollector,
		Logger:                            logger,
		IsDimmingEnabled:                  config.IsDimmerEnabled,
		ResponseTimeCollectorExcludesHTML: config.ResponseTimeCollectorExcludesHTML,
	})
	if err := server.Start(); err != nil {
		panic(fmt.Sprintf("expected server.Start() returns nil err; got err = %v", err))
	}

	// Serve the API which allows control of the reverse proxy.
	api := serving.APIServer{Server: server}
	if err := api.ListenAndServe(":8079"); err != nil {
		panic(fmt.Errorf("expected api.ListenAndServe() returns nil err; got err = %w", err))
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

func initRequestFilter() *filters.RequestFilter {
	filter := filters.NewRequestFilter()
	filter.AddPathForAllMethods("recommender")
	filter.AddPathForAllMethods("news.html")
	filter.AddPathForAllMethods("news")
	filter.AddPath("cart", "GET")
	if err := filter.AddRefererExclusion("cart", "GET", "basket.html"); err != nil {
		panic(fmt.Sprintf("expected filter.AddRefererExclusion() returns nil err; got err = %v", err))
	}
	return filter
}

func initPathProbabilities() *filters.PathProbabilities {
	// Set the defaultValue to 1 so we allow dimming by default for paths which
	// are not in the probabilities list.
	p, err := filters.NewPathProbabilities(1)
	if err != nil {
		panic(fmt.Sprintf("expected initPathProbabilities() returns nil err; got err = %v", err))
	}
	return p
}

func initPIDController(config *Config) *pidcontroller.PIDController {
	c, err := pidcontroller.NewPIDController(
		pidcontroller.NewRealtimeClock(),
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
	pid *pidcontroller.PIDController,
	responseTimeCollector responsetime.Collector,
	logger logging.Logger,
) *serving.ServerControlLoop {
	if config.ControllerPercentile != "p50" && config.ControllerPercentile != "p75" && config.ControllerPercentile != "p95" {
		log.Fatalf("expected environment variable CONTROLLER_PERCENTILE to be one of {p50|p75|p95}; got %s", config.ControllerPercentile)
	}

	c, err := serving.NewServerControlLoop(pid, responseTimeCollector, config.ControllerPercentile, logger)
	if err != nil {
		log.Fatalf("expected NewServerControlLoop() returns nil err; got err = %v", err)
	}

	return c
}
