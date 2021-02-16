package controller

import "time"

type Clock interface {
	Now() time.Time
}

type RealtimeClock struct{}

func (RealtimeClock) Now() time.Time { return time.Now() }
