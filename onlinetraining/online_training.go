package onlinetraining

import (
	"errors"
	"fmt"
	"github.com/kcz17/dimmer/filters"
	"github.com/kcz17/dimmer/logging"
	"github.com/kcz17/dimmer/responsetimecollector"
	"github.com/valyala/fasthttp"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const onlineTrainingCookieKey = "ONLINE_TRAINING"
const onlineTrainingCookieControl = "CONTROL"
const onlineTrainingCookieCandidate = "CANDIDATE"
const onlineTrainingCookieCandidateProbability = 0.05

type OnlineTraining struct {
	logger                      logging.Logger
	controlGroupResponseTimes   responsetimecollector.Collector
	candidateGroupResponseTimes responsetimecollector.Collector
	candidatePathProbabilities  *filters.PathProbabilities
	probabilitySampler          *ProbabilitySampler
	paths                       []string
	// controlPathProbabilities is a pointer to the main ("control") group
	// of path probabilities applied to the majority of requests under Server.
	controlPathProbabilities *filters.PathProbabilities
	// mux protects fields from race conditions.
	mux *sync.Mutex

	// loopStarted is used so the control loop can be started and stopped.
	loopStarted bool
	// As trainingLoop runs in a goroutine, loopWaiter and loopStop allow the
	// spawned goroutine to be gracefully stopped.
	loopWaiter *sync.WaitGroup
	loopStop   chan bool
}

func NewOnlineTraining(logger logging.Logger, paths []string, controlPathProbabilities *filters.PathProbabilities, defaultPathProbability float64) (*OnlineTraining, error) {
	candidatePathProbabilities, err := filters.NewPathProbabilities(defaultPathProbability)
	if err != nil {
		return nil, fmt.Errorf("expected filters.NewPathProbabilities() returns nil err; got err = %w", err)
	}

	return &OnlineTraining{
		logger:                      logger,
		controlGroupResponseTimes:   responsetimecollector.NewTachymeterCollector(100),
		candidateGroupResponseTimes: responsetimecollector.NewTachymeterCollector(100),
		candidatePathProbabilities:  candidatePathProbabilities,
		probabilitySampler:          NewProbabilitySampler(),
		paths:                       paths,
		controlPathProbabilities:    controlPathProbabilities,
		mux:                         &sync.Mutex{},
	}, nil
}

func (t *OnlineTraining) StartLoop() error {
	if t.loopStarted {
		return errors.New("OnlineTrainingLoop.Start() failed: training loop already started")
	}

	t.loopStop = make(chan bool, 1)
	t.loopWaiter = &sync.WaitGroup{}
	t.loopWaiter.Add(1)
	go t.trainingLoop()

	t.loopStarted = true
	return nil
}

func (t *OnlineTraining) StopLoop() error {
	if !t.loopStarted {
		return errors.New("OnlineTrainingLoop.Stop() failed: training loop not running")
	}

	// ResetCollector the control loop, response time collector and PID controller
	// in this order to ensure stale data is not written between each reset.
	close(t.loopStop)
	t.loopWaiter.Wait()
	t.candidateGroupResponseTimes.Reset()
	t.controlGroupResponseTimes.Reset()

	t.loopStarted = false
	return nil
}

func (t *OnlineTraining) trainingLoop() {
	defer t.loopWaiter.Done()
	for {
		select {
		// Stop the control loop when Stop() called.
		case <-t.loopStop:
			return
		default:
			// Sample new rules.
			newCandidateRules := t.sampleCandidateGroupProbabilities()
			t.candidatePathProbabilities.Clear()
			if err := t.candidatePathProbabilities.SetAll(newCandidateRules); err != nil {
				panic(fmt.Errorf("expected t.candidatePathProbabilities.SetAll(rules = %+v) returns nil err; got err = %w", newCandidateRules, err))
			}
			fmt.Printf("[%s] setting new candidate rules: %+v\n", time.Now().Format(time.StampMilli), newCandidateRules)

			t.candidateGroupResponseTimes.Reset()
			t.controlGroupResponseTimes.Reset()

			// Wait for enough data to be collected while continuing to listen for
			// Stop() in a non-blocking manner.
			select {
			case <-t.loopStop:
				return
			case <-time.After(2 * time.Minute):
				break
			}

			// Test whether the rules collected are significant, overriding the
			// main path probabilities if so.
			comparison := t.checkCandidateImprovesResponseTimes()
			fmt.Printf("[%s] significant reduction: %t\n", time.Now().Format(time.StampMilli), comparison)
			if comparison {
				fmt.Printf("[%s] setting control to candidate rules\n", time.Now().Format(time.StampMilli))
				t.logger.LogControlProbabilityChange(newCandidateRules)
				if err := t.controlPathProbabilities.SetAll(newCandidateRules); err != nil {
					panic(fmt.Errorf("expected t.controlPathProbabilities.SetAll(rules = %+v) returns nil err; got err = %w", err))
				}
			}
		}
	}
}

func (t *OnlineTraining) SetPaths(paths []string) {
	t.mux.Lock()
	t.paths = paths
	t.mux.Unlock()
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

func (t *OnlineTraining) sampleCandidateGroupProbabilities() []filters.PathProbabilityRule {
	t.mux.Lock()
	defer t.mux.Unlock()

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

	return rules
}

func (t *OnlineTraining) checkCandidateImprovesResponseTimes() bool {
	candidateAggregate := t.candidateGroupResponseTimes.Aggregate()
	controlAggregate := t.controlGroupResponseTimes.Aggregate()

	controlP95 := float64(controlAggregate.P95) / float64(time.Second)
	candidateP95 := float64(candidateAggregate.P95) / float64(time.Second)
	fmt.Printf("[%s] control p95: %.3f, candidate p95: %.3f \n", time.Now().Format(time.StampMilli), controlP95, candidateP95)

	// Use the heuristic that the candidate probabilities must decrease the
	// control 95th percentile by at least 5% of the control 95th percentile.
	return candidateP95 <= (controlP95 - controlP95*0.05)
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
