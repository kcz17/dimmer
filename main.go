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
	frontEndPort           string  `envconfig:"FE_PORT"`
	backEndPort            string  `envconfig:"BE_PORT"`
	requestsWindow         int     `envconfig:"NUM_REQUESTS_WINDOW"`
	controllerSamplePeriod float64 `envconfig:"CONTROLLER_SAMPLE_PERIOD"`
	controllerSetpoint     float64 `envconfig:"CONTROLLER_SETPOINT"`
	controllerKp           float64 `envconfig:"CONTROLLER_KP"`
	controllerKi           float64 `envconfig:"CONTROLLER_KI"`
	controllerKd           float64 `envconfig:"CONTROLLER_KD"`
	loggerDriver           string  `envconfig:"LOGGER_DRIVER"`
	loggerInfluxDBHost     string  `envconfig:"LOGGER_INFLUXDB_HOST"`
	loggerInfluxDBToken    string  `envconfig:"LOGGER_INFLUXDB_TOKEN"`
}

func main() {
	var config Config
	err := envconfig.Process("", &config)
	if err != nil {
		log.Fatalf("expected err == nil in envconfig.Process(); got err = %v", err)
	}

	fmt.Printf("Loaded config:\n%+v\n", config)

	var logger Logger
	if config.loggerDriver == "stdout" {
		logger = NewStdLogger()
	} else if config.loggerDriver == "influxdb" {
		logger = NewInfluxDBLogger(config.loggerInfluxDBHost, config.loggerInfluxDBToken)
	} else {
		log.Fatalf("expected env var LOGGER_DRIVER one of {stdout, influxdb}; got %s", config.loggerDriver)
	}

	tach := tachymeter.New(&tachymeter.Config{Size: config.requestsWindow})
	pid, err := controller.NewPIDController(
		controller.NewRealtimeClock(),
		config.controllerSetpoint,
		config.controllerKp,
		config.controllerKi,
		config.controllerKd,
		true,
		0,
		100,
		config.controllerSamplePeriod,
	)
	if err != nil {
		log.Fatalf("expected controller.NewPIDController() returns nil err; got err = %v", err)
	}
	go dimmer(tach, pid, logger)

	backendUrl, err := url.Parse("http://localhost:" + config.backEndPort)
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

	err = http.ListenAndServe(fmt.Sprintf(":%v", config.frontEndPort), nil)
	if err != nil {
		log.Fatalf("Error serving reverse proxy: %v", err)
	}
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
