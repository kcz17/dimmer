package main

import (
	"fmt"
	"github.com/kcz17/dimmer/controller"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/jamiealquiza/tachymeter"
)

type config struct {
	frontEndPort           string
	backEndPort            string
	requestsWindow         int
	controllerSamplePeriod float64
	controllerSetpoint     float64
	controllerKp           float64
	controllerKi           float64
	controllerKd           float64
}

func main() {
	config := loadConfig()

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
	go dimmer(tach, pid)

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

func dimmer(tach *tachymeter.Tachymeter, pid *controller.PIDController) {
	for range time.Tick(time.Second * 1) {
		metrics := tach.Calc()
		pidOutput := pid.Output(float64(metrics.Time.P95) / float64(time.Second))
		fmt.Printf("[%s] p50: %s, p95: %s, dimming: %.2f%%\n", time.Now().Format(time.StampMilli), metrics.Time.P50, metrics.Time.P95, pidOutput)
	}
}

// Loads environment variables.
func loadConfig() *config {
	fp := os.Getenv("FE_PORT")
	if fp == "" {
		log.Fatal("FE_PORT env var is missing.")
	}

	bp := os.Getenv("BE_PORT")
	if bp == "" {
		log.Fatal("BE_PORT env var is missing.")
	}

	envWindow := os.Getenv("NUM_REQUESTS_WINDOW")
	if envWindow == "" {
		log.Fatal("NUM_REQUESTS_WINDOW env var is missing.")
	}
	window, err := strconv.Atoi(envWindow)
	if err != nil {
		log.Fatal("NUM_REQUESTS_WINDOW env var cannot be converted to integer.")
	}

	envSamplePeriod := os.Getenv("CONTROLLER_SAMPLE_PERIOD")
	if envSamplePeriod == "" {
		log.Fatal("CONTROLLER_SAMPLE_PERIOD env var is missing.")
	}
	samplePeriod, err := strconv.ParseFloat(envSamplePeriod, 64)
	if err != nil {
		log.Fatal("CONTROLLER_SAMPLE_PERIOD env var cannot be converted to integer.")
	}

	envSetpoint := os.Getenv("CONTROLLER_SETPOINT")
	if envSetpoint == "" {
		log.Fatal("CONTROLLER_SETPOINT env var is missing.")
	}
	setpoint, err := strconv.ParseFloat(envSetpoint, 64)
	if err != nil {
		log.Fatal("CONTROLLER_SETPOINT env var cannot be converted to integer.")
	}

	envKp := os.Getenv("CONTROLLER_KP")
	if envKp == "" {
		log.Fatal("CONTROLLER_KP env var is missing.")
	}
	Kp, err := strconv.ParseFloat(envKp, 64)
	if err != nil {
		log.Fatal("CONTROLLER_KP env var cannot be converted to integer.")
	}

	envKi := os.Getenv("CONTROLLER_KI")
	if envKi == "" {
		log.Fatal("CONTROLLER_KI env var is missing.")
	}
	Ki, err := strconv.ParseFloat(envKi, 64)
	if err != nil {
		log.Fatal("CONTROLLER_KI env var cannot be converted to integer.")
	}

	envKd := os.Getenv("CONTROLLER_KD")
	if envKd == "" {
		log.Fatal("CONTROLLER_KD env var is missing.")
	}
	Kd, err := strconv.ParseFloat(envKd, 64)
	if err != nil {
		log.Fatal("CONTROLLER_KD env var cannot be converted to integer.")
	}

	return &config{
		frontEndPort:           fp,
		backEndPort:            bp,
		requestsWindow:         window,
		controllerSamplePeriod: samplePeriod,
		controllerSetpoint:     setpoint,
		controllerKp:           Kp,
		controllerKi:           Ki,
		controllerKd:           Kd,
	}
}
