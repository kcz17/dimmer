package onlinetraining

import (
	"fmt"
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
	controlPathProbabilities    *filters.PathProbabilities
	candidatePathProbabilities  *filters.PathProbabilities
	probabilitySampler          *ProbabilitySampler
	paths                       []string
}

func NewOnlineTraining(paths []string, defaultPathProbability float64) (*OnlineTraining, error) {
	candidatePathProbabilities, err := filters.NewPathProbabilities(defaultPathProbability)
	if err != nil {
		return nil, fmt.Errorf("expected filters.NewPathProbabilities() returns nil err; got err = %w", err)
	}

	return &OnlineTraining{
		isEnabled:                   false,
		controlGroupResponseTimes:   responsetimecollector.NewTachymeterCollector(100),
		candidateGroupResponseTimes: responsetimecollector.NewTachymeterCollector(100),
		candidatePathProbabilities:  candidatePathProbabilities,
		probabilitySampler:          NewProbabilitySampler(),
		paths:                       paths,
	}, nil
}

func (t *OnlineTraining) SetPaths(paths []string) {
	t.paths = paths
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

func (t *OnlineTraining) ResampleCandidateGroupProbabilities() error {
	// Sample a set of probabilities for use in rules. Order does not matter
	// as sampled probabilities are independent of paths.
	probabilities := t.probabilitySampler.Sample(t.candidatePathProbabilities.NumPaths())
	var rules []filters.PathProbabilityRule
	for i, path := range t.paths {
		rules = append(rules, filters.PathProbabilityRule{
			Path:        path,
			Probability: probabilities[i],
		})
	}

	t.candidatePathProbabilities.Clear()
	if err := t.candidatePathProbabilities.SetAll(rules); err != nil {
		return fmt.Errorf("expected t.candidatePathProbabilities.SetAll(rules = %+v) returns nil err; got err = %w", rules, err)
	}

	return nil
}

func (t *OnlineTraining) CheckCandidateImprovesResponseTimes() bool {
	candidateAggregate := t.candidateGroupResponseTimes.Aggregate()
	controlAggregate := t.controlGroupResponseTimes.Aggregate()

	candidateP95 := float64(candidateAggregate.P95) / float64(time.Second)
	controlP95 := float64(controlAggregate.P95) / float64(time.Second)

	// Use the heuristic that the candidate probabilities must decrease the
	// control 95th percentile by at least 5% of the control 95th percentile.
	return candidateP95 <= (controlP95 - controlP95*0.05)
}

func (t *OnlineTraining) CandidateGroupProbabilities() []filters.PathProbabilityRule {
	var rules []filters.PathProbabilityRule
	for path, probability := range t.candidatePathProbabilities.List() {
		rules = append(rules, filters.PathProbabilityRule{
			Path:        path,
			Probability: probability,
		})
	}
	return rules
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
