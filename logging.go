package main

import (
	"fmt"
	"github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"time"
)

type Logger interface {
	LogControlLoop(p50 float64, p95 float64, pidOutput float64) // LogControlLoop takes in percentiles in seconds.
}

// StdLogger logs the output to standard output.
type StdLogger struct{}

func NewStdLogger() *StdLogger {
	return &StdLogger{}
}

func (*StdLogger) LogControlLoop(p50 float64, p95 float64, pidOutput float64) {
	fmt.Printf("[%s] p50: %.3f, p95: %.3f, dimming: %.2f%%\n", time.Now().Format(time.StampMilli), p50, p95, pidOutput)
}

// InfluxDBLogger logs the output to an external InfluxDB instance.
type InfluxDBLogger struct {
	client      influxdb2.Client
	asyncWriter api.WriteAPI
}

func NewInfluxDBLogger(baseURL string, authToken string) *InfluxDBLogger {
	client := influxdb2.NewClient(baseURL, authToken)
	writeAPI := client.WriteAPI("kcz17", "dimmer")

	// Create a goroutine for reading and logging async write errors.
	errorsCh := writeAPI.Errors()
	go func() {
		for err := range errorsCh {
			fmt.Printf("[%s] influxdb2 async write error: %v\n", time.Now().Format(time.StampMilli), err)
		}
	}()

	return &InfluxDBLogger{
		client:      client,
		asyncWriter: writeAPI,
	}
}

func (l *InfluxDBLogger) LogControlLoop(p50 float64, p95 float64, pidOutput float64) {
	p := influxdb2.NewPointWithMeasurement("loop").
		AddField("p50", p50).
		AddField("p95", p95).
		AddField("pid_output", pidOutput).
		SetTime(time.Now())
	l.asyncWriter.WritePoint(p)
}
