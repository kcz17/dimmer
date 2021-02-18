package controller

import "time"

type Clock interface {
	Now() time.Time
}

type RealtimeClock struct{}

func NewRealtimeClock() RealtimeClock {
	return RealtimeClock{}
}

func (RealtimeClock) Now() time.Time { return time.Now() }
