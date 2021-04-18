package profiling

type Priority int

const (
	Unknown Priority = 0
	Low              = 1
	High             = 2
)

func (p Priority) String() string {
	return [...]string{"unknown", "low", "high"}[p]
}
