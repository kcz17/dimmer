package main

import (
	"fmt"
	"github.com/kcz17/dimmer/controller"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/jamiealquiza/tachymeter"
	"github.com/valyala/fasthttp"
)

type Config struct {
	FrontEndPort           string  `env:"FE_PORT"`
	BackEndPort            string  `env:"BE_PORT"`
	IsDimmerEnabled        bool    `env:"DIMMER_ENABLED" env-default:"true"`
	RequestsWindow         int     `env:"NUM_REQUESTS_WINDOW"`
	ControllerSamplePeriod float64 `env:"CONTROLLER_SAMPLE_PERIOD"`
	ControllerPercentile   string  `env:"CONTROLLER_PERCENTILE" env-default:"p75"`
	ControllerSetpoint     float64 `env:"CONTROLLER_SETPOINT"`
	ControllerKp           float64 `env:"CONTROLLER_KP"`
	ControllerKi           float64 `env:"CONTROLLER_KI"`
	ControllerKd           float64 `env:"CONTROLLER_KD"`
	LoggerDriver           string  `env:"LOGGER_DRIVER"`
	LoggerInfluxDBHost     string  `env:"LOGGER_INFLUXDB_HOST"`
	LoggerInfluxDBToken    string  `env:"LOGGER_INFLUXDB_TOKEN"`
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
	tach := tachymeter.New(&tachymeter.Config{Size: config.RequestsWindow})
	pid, err := controller.NewPIDController(
		controller.NewRealtimeClock(),
		config.ControllerSetpoint,
		config.ControllerKp,
		config.ControllerKi,
		config.ControllerKd,
		true,
		0,
		99,
		config.ControllerSamplePeriod,
	)
	if err != nil {
		log.Fatalf("expected controller.NewPIDController() returns nil err; got err = %v", err)
	}

	controllerOutputMux := &sync.RWMutex{}
	controllerOutput := 0.0
	go controlLoop(tach, pid, logger, config.ControllerPercentile, &controllerOutput, controllerOutputMux)

	proxy := &fasthttp.HostClient{
		Addr: "localhost:" + config.BackEndPort,
	}

	if err := fasthttp.ListenAndServe(fmt.Sprintf(":%v", config.FrontEndPort), func(ctx *fasthttp.RequestCtx) {
		req := &ctx.Request
		resp := &ctx.Response

		if config.IsDimmerEnabled && requestFilter.Matches(string(ctx.Path()), string(ctx.Method())) {
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

		startTime := time.Now()
		if err := proxy.Do(req, resp); err != nil {
			ctx.Logger().Printf("fasthttp: error when proxying the request: %v", err)
		}
		duration := time.Now().Sub(startTime)
		tach.AddTime(duration)

		// Remove connection header per RFC2616.
		func(resp *fasthttp.Response) {
			resp.Header.Del("Connection")
		}(resp)
	}); err != nil {
		log.Fatalf("fasthttp: server error: %v", err)
	}
}

func initLogger(config *Config) Logger {
	var logger Logger
	if config.LoggerDriver == "stdout" {
		logger = NewStdLogger()
	} else if config.LoggerDriver == "influxdb" {
		logger = NewInfluxDBLogger(config.LoggerInfluxDBHost, config.LoggerInfluxDBToken)
	} else {
		log.Fatalf("expected env var LOGGER_DRIVER one of {stdout, influxdb}; got %s", config.LoggerDriver)
	}
	return logger
}

func initRequestFilter() *RequestFilter {
	filter := NewRequestFilter()
	filter.AddPathForAllMethods("recommender")
	filter.AddPathForAllMethods("news.html")
	filter.AddPathForAllMethods("news")
	filter.AddPathForAllMethods("cart")
	return filter
}

func controlLoop(tach *tachymeter.Tachymeter, pid *controller.PIDController, logger Logger, dimmingPercentile string, dimmingPercentage *float64, dimmingPercentageMux *sync.RWMutex) {
	for range time.Tick(time.Second * 1) {
		metrics := tach.Calc()

		// PID controller and logger operate with seconds.
		p50 := float64(metrics.Time.P50) / float64(time.Second)
		p75 := float64(metrics.Time.P75) / float64(time.Second)
		p95 := float64(metrics.Time.P95) / float64(time.Second)
		logger.LogResponseTime(p50, p75, p95)

		var pidOutput float64
		if dimmingPercentile == "p50" {
			pidOutput = pid.Output(p50)
		} else if dimmingPercentile == "p75" {
			pidOutput = pid.Output(p75)
		} else if dimmingPercentile == "p95" {
			pidOutput = pid.Output(p95)
		} else {
			log.Fatalf("controlLoop() expected dimmingPercentile to be one of {50|75|90|95}; got %s", dimmingPercentile)
		}
		logger.LogDimmerOutput(pidOutput)
		logger.LogPIDControllerState(pid.DebugP, pid.DebugI, pid.DebugD, pid.DebugErr)

		// Apply the PID output.
		dimmingPercentageMux.Lock()
		*dimmingPercentage = pidOutput
		dimmingPercentageMux.Unlock()
	}
}
