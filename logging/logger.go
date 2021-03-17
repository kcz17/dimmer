package logging

import "github.com/kcz17/dimmer/filters"

type Logger interface {
	LogResponseTime(t float64)                                       // Takes in response time in seconds.
	LogAggregateResponseTimes(p50 float64, p75 float64, p95 float64) // Takes in percentiles in seconds.
	LogDimmerOutput(pidOutput float64)
	LogPIDControllerState(p float64, i float64, d float64, errorTerm float64)
	LogControlProbabilityChange(probabilities []filters.PathProbabilityRule)
}

// noopLogger does not perform any logging.
type noopLogger struct{}

func NewNoopLogger() *noopLogger {
	return &noopLogger{}
}

func (*noopLogger) LogResponseTime(float64) {
	return
}

func (*noopLogger) LogAggregateResponseTimes(float64, float64, float64) {
	return
}

func (*noopLogger) LogDimmerOutput(float64) {
	return
}

func (*noopLogger) LogPIDControllerState(float64, float64, float64, float64) {
	return
}

func (*noopLogger) LogControlProbabilityChange([]filters.PathProbabilityRule) {
	return
}
