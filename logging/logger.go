package logging

type Logger interface {
	LogResponseTime(t float64)                                       // Takes in response time in seconds.
	LogAggregateResponseTimes(p50 float64, p75 float64, p95 float64) // Takes in percentiles in seconds.
	LogDimmerOutput(pidOutput float64)
	LogPIDControllerState(p float64, i float64, d float64, errorTerm float64)
	LogOnlineTrainingProbabilities(control map[string]float64, candidate map[string]float64)
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

func (*noopLogger) LogOnlineTrainingProbabilities(map[string]float64, map[string]float64) {
	return
}
