package logging

import (
	"fmt"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"time"
)

// influxDBLogger logs the output to an external InfluxDB instance.
type influxDBLogger struct {
	client      influxdb2.Client
	asyncWriter api.WriteAPI
}

func NewInfluxDBLogger(baseURL string, authToken string) *influxDBLogger {
	options := influxdb2.DefaultOptions()
	options.WriteOptions().SetBatchSize(1000)
	options.WriteOptions().SetFlushInterval(250)

	client := influxdb2.NewClientWithOptions(baseURL, authToken, options)
	writeAPI := client.WriteAPI("kcz17", "dimmer")

	// Create a goroutine for reading and logging async write errors.
	errorsCh := writeAPI.Errors()
	go func() {
		for err := range errorsCh {
			fmt.Printf("[%s] influxdb2 async write error: %v\n", time.Now().Format(time.StampMilli), err)
		}
	}()

	return &influxDBLogger{
		client:      client,
		asyncWriter: writeAPI,
	}
}

func (l *influxDBLogger) LogResponseTime(t float64) {
	p := influxdb2.NewPointWithMeasurement("dimmer_individual_response_time").
		AddField("t", t).
		SetTime(time.Now())
	l.asyncWriter.WritePoint(p)
}

func (l *influxDBLogger) LogAggregateResponseTimes(p50 float64, p75 float64, p95 float64) {
	p := influxdb2.NewPointWithMeasurement("dimmer_response_time").
		AddField("p50", p50).
		AddField("p75", p75).
		AddField("p95", p95).
		SetTime(time.Now())
	l.asyncWriter.WritePoint(p)
}

func (l *influxDBLogger) LogDimmerOutput(pidOutput float64) {
	p := influxdb2.NewPointWithMeasurement("dimmer_output").
		AddField("output", pidOutput).
		SetTime(time.Now())
	l.asyncWriter.WritePoint(p)
}

func (l *influxDBLogger) LogPIDControllerState(p float64, i float64, d float64, errorTerm float64) {
	point := influxdb2.NewPointWithMeasurement("dimmer_pid_controller_state").
		AddField("p", p).
		AddField("i", i).
		AddField("d", d).
		AddField("e_t", errorTerm).
		SetTime(time.Now())
	l.asyncWriter.WritePoint(point)
}
