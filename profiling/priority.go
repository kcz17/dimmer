package profiling

import "fmt"

type Priority int

const (
	Unknown Priority = 0
	Low              = 1
	High             = 2
)

func strToPriority(str string) (Priority, error) {
	if str == "unknown" {
		return Unknown, nil
	} else if str == "low" {
		return Low, nil
	} else if str == "high" {
		return High, nil
	} else {
		return Unknown, fmt.Errorf("unknown priority string %s", str)
	}
}

func (p Priority) String() string {
	return [...]string{"unknown", "low", "high"}[p]
}
