package main

import (
	"fmt"
	"time"
)

type Logger interface {
	LogControlLoop(p50 float64, p95 float64, pidOutput float64)
}

// StdLogger logs the output to standard output.
type StdLogger struct{}

func NewStdLogger() *StdLogger {
	return &StdLogger{}
}

// LogControlLoop takes in percentiles in seconds.
func (*StdLogger) LogControlLoop(p50 float64, p95 float64, pidOutput float64) {
	fmt.Printf("[%s] p50: %.3f, p95: %.3f, dimming: %.2f%%\n", time.Now().Format(time.StampMilli), p50, p95, pidOutput)
}
