package onlinetraining

import (
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/responsetimecollector"
	"github.com/valyala/fasthttp"
	"math/rand"
	"strings"
	"time"
)

const onlineTrainingCookieKey = "ONLINE_TRAINING"
const onlineTrainingCookieControl = "CONTROL"
const onlineTrainingCookieCandidate = "CANDIDATE"
const onlineTrainingCookieCandidateProbability = 0.05

type OnlineTraining struct {
	isEnabled                   bool
	controlGroupResponseTimes   responsetimecollector.Collector
	candidateGroupResponseTimes responsetimecollector.Collector
	candidatePathProbabilities  *filters.PathProbabilities
}

func NewOnlineTraining(pathProbabilities *filters.PathProbabilities) *OnlineTraining {
	return &OnlineTraining{
		isEnabled:                   false,
		controlGroupResponseTimes:   responsetimecollector.NewTachymeterCollector(100),
		candidateGroupResponseTimes: responsetimecollector.NewTachymeterCollector(100),
		candidatePathProbabilities:  pathProbabilities,
	}
}

func (t *OnlineTraining) IsEnabled() bool {
	return t.isEnabled
}

func (t *OnlineTraining) SampleCandidateGroupShouldDim(path string) bool {
	return t.candidatePathProbabilities.SampleShouldDim(path)
}

func (t *OnlineTraining) AddCandidateResponseTime(duration time.Duration) {
	t.candidateGroupResponseTimes.Add(duration)
}

func (t *OnlineTraining) AddControlResponseTime(duration time.Duration) {
	t.controlGroupResponseTimes.Add(duration)
}

func RequestHasCookie(request *fasthttp.Request) bool {
	return len(request.Header.Cookie(onlineTrainingCookieKey)) == 0
}

func RequestHasCandidateCookie(request *fasthttp.Request) bool {
	return strings.Compare(onlineTrainingCookieCandidate,
		string(request.Header.Cookie(onlineTrainingCookieKey))) == 0
}

func SampleCookie() *fasthttp.Cookie {
	if rand.Float64() < onlineTrainingCookieCandidateProbability {
		return candidateCookie()
	} else {
		return controlCookie()
	}
}

func controlCookie() *fasthttp.Cookie {
	cookie := &fasthttp.Cookie{}
	cookie.SetKey(onlineTrainingCookieKey)
	cookie.SetValue(onlineTrainingCookieControl)
	return cookie
}

func candidateCookie() *fasthttp.Cookie {
	cookie := &fasthttp.Cookie{}
	cookie.SetKey(onlineTrainingCookieKey)
	cookie.SetValue(onlineTrainingCookieControl)
	return cookie
}
