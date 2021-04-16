package profiling

type Profiler struct {
	Priorities PriorityFetcher
	Requests   RequestWriter
}
