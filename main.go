package main

import (
	"fmt"
	"github.com/kcz17/dimmer/config"
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/offlinetraining"
	"github.com/kcz17/dimmer/onlinetraining"
	"github.com/kcz17/dimmer/pid"
	"github.com/kcz17/dimmer/profiling"
	"github.com/kcz17/dimmer/responsetimecollector"
	"log"
	"strconv"
)

// ResponseTimeCollectorRequestsWindow defines the number of requests from which
// the aggregate response time will be calculated for the PID control loop. As
// the control loop runs every second by default, this is set to 100, hence
// requiring a minimum of 100rps for non-negligible response times to be passed
// to the PID controller. This is justifiable as web servers tend to encounter
// load above 100rps.
const ResponseTimeCollectorRequestsWindow = 100

func main() {
	conf := config.ReadConfig()

	logger := initLogger(conf)

	controlLoop := initControlLoop(
		conf,
		initPIDController(conf),
		responsetimecollector.NewTachymeterCollector(ResponseTimeCollectorRequestsWindow),
		logger,
	)

	// Filters used to selectively dim routes.
	requestFilter := initRequestFilter(conf)
	pathProbabilities := initPathProbabilities(conf)

	onlineTrainingService, err := onlinetraining.NewOnlineTraining(
		logger,
		initPaths(conf),
		pathProbabilities,
		1,
	)
	if err != nil {
		log.Fatalf("expected onlineTrainingService to return nil err; got err = %v", err)
	}

	var profiler *profiling.Profiler
	if *conf.Dimming.Profiler.Enabled {
		priorityFetcher, err := profiling.NewRedisPriorityFetcher(
			*conf.Dimming.Profiler.Redis.Addr,
			*conf.Dimming.Profiler.Redis.Password,
			*conf.Dimming.Profiler.Redis.PrioritiesDB,
			*conf.Dimming.Profiler.Redis.QueueDB,
		)
		if err != nil {
			panic(fmt.Errorf("could not create RedisPriorityFetcher: %w", err))
		}

		profiler = &profiling.Profiler{
			Priorities: priorityFetcher,
			Requests: profiling.NewInfluxDBRequestWriter(
				*conf.Dimming.Profiler.InfluxDB.Host,
				*conf.Dimming.Profiler.InfluxDB.Token,
				*conf.Dimming.Profiler.InfluxDB.Org,
				*conf.Dimming.Profiler.InfluxDB.Bucket,
			),
			Aggregator:                               profiling.NewProfiledRequestAggregator(),
			LowPriorityDimmingProbability:            *conf.Dimming.Profiler.Probabilities.Low,
			LowPriorityDimmingProbabilityMultiplier:  *conf.Dimming.Profiler.Probabilities.LowMultiplier,
			HighPriorityDimmingProbability:           *conf.Dimming.Profiler.Probabilities.High,
			HighPriorityDimmingProbabilityMultiplier: *conf.Dimming.Profiler.Probabilities.HighMultiplier,
		}
	}

	// Serve the reverse proxy with dimming control loop.
	server := NewServer(&ServerOptions{
		FrontendAddr:           fmt.Sprintf(":%v", conf.Proxying.FrontendPort),
		BackendAddr:            *conf.Proxying.BackendHost + ":" + strconv.Itoa(*conf.Proxying.BackendPort),
		MaxConns:               2048,
		ControlLoop:            controlLoop,
		RequestFilter:          requestFilter,
		PathProbabilities:      pathProbabilities,
		Logger:                 logger,
		IsDimmingEnabled:       *conf.Dimming.Enabled,
		OnlineTrainingService:  onlineTrainingService,
		OfflineTrainingService: offlinetraining.NewOfflineTraining(),
		IsProfilingEnabled:     *conf.Dimming.Profiler.Enabled,
		ProfilingService:       profiler,
		ProfilingSessionCookie: *conf.Dimming.Profiler.SessionCookie,
	})

	// Start the server in a goroutine so we can separately block the main
	// thread on the following API server.
	go func() {
		if err := server.ListenAndServe(); err != nil {
			panic(fmt.Sprintf("expected server.ListenAndServe() returns nil err; got err = %v", err))
		}
	}()

	api := APIServer{Server: server}
	if err := api.ListenAndServe(":8079"); err != nil {
		panic(fmt.Errorf("expected api.ListenAndServe() returns nil err; got err = %w", err))
	}
}

func initLogger(conf *config.Config) logging.Logger {
	var logger logging.Logger
	if *conf.Logging.Driver == "noop" {
		logger = logging.NewNoopLogger()
	} else if *conf.Logging.Driver == "stdout" {
		logger = logging.NewStdoutLogger()
	} else if *conf.Logging.Driver == "influxdb" {
		logger = logging.NewInfluxDBLogger(
			*conf.Logging.InfluxDB.Host,
			*conf.Logging.InfluxDB.Token,
			*conf.Logging.InfluxDB.Org,
			*conf.Logging.InfluxDB.Bucket,
		)
	} else {
		log.Fatalf("expected env var LOGGER_DRIVER one of {noop, stdout, influxdb}; got %s", conf.Logging.Driver)
	}
	return logger
}

func initPaths(conf *config.Config) []string {
	var paths []string
	for _, component := range conf.Dimming.DimmableComponents {
		paths = append(paths, *component.Path)
	}
	return paths
}

func initRequestFilter(conf *config.Config) *filters.RequestFilter {
	filter := filters.NewRequestFilter()
	for _, component := range conf.Dimming.DimmableComponents {
		if *component.Method.ShouldMatchAll {
			filter.AddPathForAllMethods(*component.Path)
		} else {
			filter.AddPath(*component.Path, *component.Method.Method)
		}

		for _, exclusion := range component.Exclusions {
			if err := filter.AddRefererExclusion(*component.Path, *exclusion.Method, *exclusion.Substring); err != nil {
				log.Fatalf("expected filter.AddRefererExclusion(path=%s, method=%s, substring=%s) returns nil err; got err = %v", component.Path, exclusion.Method, exclusion.Substring, err)
			}
		}
	}
	return filter
}

func initPathProbabilities(conf *config.Config) *filters.PathProbabilities {
	// Set the defaultValue to 1 so we allow dimming by default for paths which
	// are not in the probabilities list.
	p, err := filters.NewPathProbabilities(1)
	if err != nil {
		panic(fmt.Sprintf("expected initPathProbabilities() returns nil err; got err = %v", err))
	}

	for _, component := range conf.Dimming.DimmableComponents {
		if component.Probability != nil {
			rule := filters.PathProbabilityRule{
				Path:        *component.Path,
				Probability: *component.Probability,
			}

			if err := p.Set(rule); err != nil {
				log.Fatalf("expected PathProbabilities.Set(rule=%+v) returns nil err; got err = %v", rule, err)
			}
		}
	}

	return p
}

func initPIDController(conf *config.Config) *pid.PIDController {
	c, err := pid.NewPIDController(
		pid.NewRealtimeClock(),
		*conf.Dimming.Controller.Setpoint,
		*conf.Dimming.Controller.Kp,
		*conf.Dimming.Controller.Ki,
		*conf.Dimming.Controller.Kd,
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
		*conf.Dimming.Controller.SamplePeriod,
	)
	if err != nil {
		log.Fatalf("expected controller.NewPIDController() returns nil err; got err = %v", err)
	}

	return c
}

func initControlLoop(
	conf *config.Config,
	pid *pid.PIDController,
	responseTimeCollector responsetimecollector.Collector,
	logger logging.Logger,
) *ServerControlLoop {
	percentile := *conf.Dimming.Controller.Percentile
	if percentile != "p50" && percentile != "p75" && percentile != "p95" {
		log.Fatalf("expected environment variable CONTROLLER_PERCENTILE to be one of {p50|p75|p95}; got %s", percentile)
	}

	c, err := NewServerControlLoop(pid, responseTimeCollector, percentile, logger)
	if err != nil {
		log.Fatalf("expected NewServerControlLoop() returns nil err; got err = %v", err)
	}

	return c
}
