package main

import (
	"fmt"
	"github.com/kcz17/dimmer/controller"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/jamiealquiza/tachymeter"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	FrontEndPort           string  `envconfig:"FE_PORT"`
	BackEndPort            string  `envconfig:"BE_PORT"`
	RequestsWindow         int     `envconfig:"NUM_REQUESTS_WINDOW"`
	ControllerSamplePeriod float64 `envconfig:"CONTROLLER_SAMPLE_PERIOD"`
	ControllerSetpoint     float64 `envconfig:"CONTROLLER_SETPOINT"`
	ControllerKp           float64 `envconfig:"CONTROLLER_KP"`
	ControllerKi           float64 `envconfig:"CONTROLLER_KI"`
	ControllerKd           float64 `envconfig:"CONTROLLER_KD"`
	LoggerDriver           string  `envconfig:"LOGGER_DRIVER"`
	LoggerInfluxDBHost     string  `envconfig:"LOGGER_INFLUXDB_HOST"`
	LoggerInfluxDBToken    string  `envconfig:"LOGGER_INFLUXDB_TOKEN"`
}

func main() {
	var config Config
	err := envconfig.Process("", &config)
	if err != nil {
		log.Fatalf("expected err == nil in envconfig.Process(); got err = %v", err)
	}

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
		100,
		config.ControllerSamplePeriod,
	)
	if err != nil {
		log.Fatalf("expected controller.NewPIDController() returns nil err; got err = %v", err)
	}
	go dimmer(tach, pid, logger)

	backendUrl, err := url.Parse("http://localhost:" + config.BackEndPort)
	if err != nil {
		log.Fatalf("Error parsing backend url: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendUrl)
	http.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
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

func dimmer(tach *tachymeter.Tachymeter, pid *controller.PIDController, logger Logger) {
	for range time.Tick(time.Second * 1) {
		metrics := tach.Calc()

		// PID controller and logger operate with seconds.
		p50 := float64(metrics.Time.P50) / float64(time.Second)
		p95 := float64(metrics.Time.P95) / float64(time.Second)
		pidOutput := pid.Output(p95)
		logger.LogControlLoop(p50, p95, pidOutput)
	}
}
