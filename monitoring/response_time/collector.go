package response_time

import "time"

type ResponseTimeAggregation struct {
	P50 time.Duration // 50th percentile response time in second.
	P75 time.Duration // 75th percentile response time in second.
	P95 time.Duration // 95th percentile response time in second.
}

type ResponseTimeCollector interface {
	Add(t time.Duration)                 // Add sends a new response time to the collector.
	Aggregate() *ResponseTimeAggregation // Aggregate calculates aggregate metrics over a defined time period.
}
