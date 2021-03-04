package logging

import (
	"fmt"
	"time"
)

// stdoutLogger logs the output to standard output.
type stdoutLogger struct{}

func NewStdoutLogger() *stdoutLogger {
	return &stdoutLogger{}
}

func (*stdoutLogger) LogResponseTime(_ float64) {
	// Do not log non-aggregated response times to stdout.
	return
}

func (*stdoutLogger) LogAggregateResponseTimes(p50 float64, p75 float64, p95 float64) {
	fmt.Printf("[%s] p50: %.3f, p75: %.3f, p95: %.3f\n", time.Now().Format(time.StampMilli), p50, p75, p95)
}

func (*stdoutLogger) LogDimmerOutput(pidOutput float64) {
	fmt.Printf("[%s] dimmer output: %.2f%%\n", time.Now().Format(time.StampMilli), pidOutput)
}

func (*stdoutLogger) LogPIDControllerState(p float64, i float64, d float64, errorTerm float64) {
	fmt.Printf("[%s] p: %.3f, i: %.3f, d: %.3f, e(t): %.3f\n", time.Now().Format(time.StampMilli), p, i, d, errorTerm)
}
