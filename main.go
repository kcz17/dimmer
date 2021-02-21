package main

import (
	"fmt"
	"github.com/kcz17/dimmer/controller"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/jamiealquiza/tachymeter"
)

type Config struct {
	FrontEndPort           string  `env:"FE_PORT"`
	BackEndPort            string  `env:"BE_PORT"`
	RequestsWindow         int     `env:"NUM_REQUESTS_WINDOW"`
	IsDimmingEnabled       bool    `env:"DIMMER_ENABLED" env-default:"true"`
	ControllerSamplePeriod float64 `env:"CONTROLLER_SAMPLE_PERIOD"`
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
	go controlLoop(tach, pid, logger, &controllerOutput, controllerOutputMux)

	backendUrl, err := url.Parse("http://localhost:" + config.BackEndPort)
	if err != nil {
		log.Fatalf("Error parsing backend url: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendUrl)
	http.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		if config.IsDimmingEnabled && requestFilter.Matches(req.URL.Path, req.Method) {
			controllerOutputMux.RLock()
			dimmingPercentage := controllerOutput
			controllerOutputMux.RUnlock()

			if rand.Float64()*100 < dimmingPercentage {
				http.Error(rw, "dimming", http.StatusTooManyRequests)
				return
			}
		}

		startTime := time.Now()
		proxy.ServeHTTP(rw, req)
		duration := time.Now().Sub(startTime)
		tach.AddTime(duration)
	})

	err = http.ListenAndServe(fmt.Sprintf(":%v", config.FrontEndPort), nil)
	if err != nil {
		log.Fatalf("Error serving reverse proxy: %v", err)
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

func controlLoop(tach *tachymeter.Tachymeter, pid *controller.PIDController, logger Logger, dimmingPercentage *float64, dimmingPercentageMux *sync.RWMutex) {
	for range time.Tick(time.Second * 1) {
		metrics := tach.Calc()

		// PID controller and logger operate with seconds.
		p50 := float64(metrics.Time.P50) / float64(time.Second)
		p95 := float64(metrics.Time.P95) / float64(time.Second)
		pidOutput := pid.Output(p95)
		logger.LogControlLoop(p50, p95, pidOutput)

		// Apply the PID output.
		dimmingPercentageMux.Lock()
		*dimmingPercentage = pidOutput
		dimmingPercentageMux.Unlock()
	}
}
