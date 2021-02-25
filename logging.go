package main

import (
	"fmt"
	"github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"time"
)

type Logger interface {
	LogResponseTime(t float64)                                       // Takes in response time in seconds.
	LogAggregateResponseTimes(p50 float64, p75 float64, p95 float64) // Takes in percentiles in seconds.
	LogDimmerOutput(pidOutput float64)
	LogPIDControllerState(p float64, i float64, d float64, errorTerm float64)
}

// StdLogger logs the output to standard output.
type StdLogger struct{}

func NewStdLogger() *StdLogger {
	return &StdLogger{}
}

func (*StdLogger) LogResponseTime(_ float64) {
	// Do not log non-aggregated response times to stdout.
	return
}

func (*StdLogger) LogAggregateResponseTimes(p50 float64, p75 float64, p95 float64) {
	fmt.Printf("[%s] p50: %.3f, p75: %.3f, p95: %.3f\n", time.Now().Format(time.StampMilli), p50, p75, p95)
}

func (*StdLogger) LogDimmerOutput(pidOutput float64) {
	fmt.Printf("[%s] dimmer output: %.2f%%\n", time.Now().Format(time.StampMilli), pidOutput)
}

func (*StdLogger) LogPIDControllerState(p float64, i float64, d float64, errorTerm float64) {
	fmt.Printf("[%s] p: %.3f, i: %.3f, d: %.3f, e(t): %.3f\n", time.Now().Format(time.StampMilli), p, i, d, errorTerm)
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

func (l *InfluxDBLogger) LogResponseTime(t float64) {
	p := influxdb2.NewPointWithMeasurement("dimmer_individual_response_time").
		AddField("t", t).
		SetTime(time.Now())
	l.asyncWriter.WritePoint(p)
}

func (l *InfluxDBLogger) LogAggregateResponseTimes(p50 float64, p75 float64, p95 float64) {
	p := influxdb2.NewPointWithMeasurement("dimmer_response_time").
		AddField("p50", p50).
		AddField("p75", p75).
		AddField("p95", p95).
		SetTime(time.Now())
	l.asyncWriter.WritePoint(p)
}

func (l *InfluxDBLogger) LogDimmerOutput(pidOutput float64) {
	p := influxdb2.NewPointWithMeasurement("dimmer_output").
		AddField("output", pidOutput).
		SetTime(time.Now())
	l.asyncWriter.WritePoint(p)
}

func (l *InfluxDBLogger) LogPIDControllerState(p float64, i float64, d float64, errorTerm float64) {
	point := influxdb2.NewPointWithMeasurement("dimmer_pid_controller_state").
		AddField("p", p).
		AddField("i", i).
		AddField("d", d).
		AddField("e_t", errorTerm).
		SetTime(time.Now())
	l.asyncWriter.WritePoint(point)
}
