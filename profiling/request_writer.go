package profiling

import (
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"log"
	"time"
)

type RequestWriter interface {
	// Write logs a session request allowing the session behaviour to be
	// profiled.
	Write(sessionID string, method string, path string)
}

type InfluxDBRequestWriter struct {
	client      influxdb2.Client
	asyncWriter api.WriteAPI
}

func NewInfluxDBRequestWriter(addr, authToken, org, bucket string) *InfluxDBRequestWriter {
	options := influxdb2.DefaultOptions()
	options.WriteOptions().SetBatchSize(500)
	options.WriteOptions().SetFlushInterval(1000)

	client := influxdb2.NewClientWithOptions(addr, authToken, options)
	writeAPI := client.WriteAPI(org, bucket)

	// Create a goroutine for reading and logging async write errors.
	errorsCh := writeAPI.Errors()
	go func() {
		for err := range errorsCh {
			log.Printf("influxdb2 profiling async write error: %v\n", err)
		}
	}()

	return &InfluxDBRequestWriter{
		client:      client,
		asyncWriter: writeAPI,
	}
}

func (w *InfluxDBRequestWriter) Write(sessionID string, method string, path string) {
	p := influxdb2.NewPointWithMeasurement("request").
		AddTag("session_id", sessionID).
		AddField("method", method).
		AddField("path", path).
		SetTime(time.Now())
	w.asyncWriter.WritePoint(p)
}
