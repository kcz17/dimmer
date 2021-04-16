package profiling

type Priority int

const (
	Unknown Priority = iota
	Low
	High
)

func (p Priority) String() string {
	return [...]string{"unknown", "low", "high"}[p]
}
