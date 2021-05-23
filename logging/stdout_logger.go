package logging

import (
	"log"
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
	log.Printf("p50: %.3f, p75: %.3f, p95: %.3f\n", p50, p75, p95)
}

func (*stdoutLogger) LogDimmerOutput(pidOutput float64) {
	log.Printf("dimmer output: %.2f%%\n", pidOutput)
}

func (*stdoutLogger) LogPIDControllerState(p float64, i float64, d float64, errorTerm float64) {
	log.Printf("p: %.3f, i: %.3f, d: %.3f, e(t): %.3f\n", p, i, d, errorTerm)
}

func (*stdoutLogger) LogOnlineTrainingProbabilities(control map[string]float64, candidate map[string]float64) {
	log.Printf("online training probabilities:\n\tcontrol: %+v\n\tcandidate: %+v\n", control, candidate)
}
