package profiling

import (
	"github.com/valyala/fasthttp"
	"log"
	"time"
)

const priorityKey = "PRIORITY"
const priorityUnknownValue = "unknown"
const priorityLowValue = "low"
const priorityHighValue = "high"
const cookieUnknownDefaultExpiry = 2 * time.Minute
const cookiePriorityDefaultExpiry = 2 * time.Hour

const dimmingDecisionKey = "DIMMING_DECISION"
const dimmingDecisionTrueValue = "true"
const dimmingDecisionFalseValue = "false"
const cookieDimmingDefaultExpiry = 1 * time.Minute

// Probabilities must add up to one.
const lowPriorityDimmingProbability = 0.99
const highPriorityDimmingProbability = 0.01

type Profiler struct {
	Priorities PriorityFetcher
	Requests   RequestWriter
	Aggregator *ProfiledRequestAggregator
}

func RequestHasPriorityCookie(request *fasthttp.Request) bool {
	return len(string(request.Header.Cookie(priorityKey))) != 0
}

func RequestHasPriorityLowOrHighCookie(request *fasthttp.Request) bool {
	return string(request.Header.Cookie(priorityKey)) == priorityLowValue ||
		string(request.Header.Cookie(priorityKey)) == priorityHighValue
}

func (p *Profiler) MarkProfiledRequestByPriorityCookie(request *fasthttp.Request) {
	if string(request.Header.Cookie(priorityKey)) == priorityLowValue {
		p.Aggregator.MarkLowPriorityVisit()
	} else {
		p.Aggregator.MarkHighPriorityVisit()
	}
}

func CookieForPriority(priority Priority) *fasthttp.Cookie {
	cookie := &fasthttp.Cookie{}
	cookie.SetKey(priorityKey)
	if priority == Low {
		cookie.SetValue(priorityLowValue)
	} else if priority == High {
		cookie.SetValue(priorityHighValue)
	} else if priority == Unknown {
		cookie.SetValue(priorityUnknownValue)
	} else {
		log.Printf("unexpected priority cookie value during CookieForPriority(); priority = %d", priority)
		cookie.SetValue(priorityUnknownValue)
	}

	if priority == Low || priority == High {
		cookie.SetExpire(time.Now().Add(cookiePriorityDefaultExpiry))
	} else {
		cookie.SetExpire(time.Now().Add(cookieUnknownDefaultExpiry))
	}

	return cookie
}

func (p *Profiler) DimmingDecisionProbabilityForPriorityCookie(request *fasthttp.Request) float64 {
	// Instead of directly returning [low/high]PriorityDimmingProbability, the
	// proportion of low to high priority request must be taken into account, so
	// that, for example, the dimming decision probability of high priority
	// requests goes to 1 if there are no low priority requests to dim.
	numLow := float64(p.Aggregator.GetLowPriorityVisits())
	numHigh := float64(p.Aggregator.GetHighPriorityVisits())

	// Occurrences are incremented by one to prevent divide-by-zero errors later.
	expectation := lowPriorityDimmingProbability*(numLow+1) + highPriorityDimmingProbability*(numHigh+1)
	if string(request.Header.Cookie(priorityKey)) == priorityLowValue {
		return numLow * (numLow / expectation)
	} else if string(request.Header.Cookie(priorityKey)) == priorityHighValue {
		return numHigh * (numHigh / expectation)
	} else {
		log.Printf("unexpected priority cookie value during SampleDimmingForPriorityCookie: %s", string(request.Header.Cookie(priorityKey)))
		return 0
	}
}

func HasDimmingDecisionCookie(request *fasthttp.Request) bool {
	return len(request.Header.Cookie(dimmingDecisionKey)) != 0
}

func ReadDimmingDecisionCookie(request *fasthttp.Request) bool {
	return string(request.Header.Cookie(dimmingDecisionKey)) == dimmingDecisionTrueValue
}

func CookieForDimmingDecision(decision bool) *fasthttp.Cookie {
	cookie := &fasthttp.Cookie{}
	cookie.SetKey(dimmingDecisionKey)
	if decision {
		cookie.SetValue(dimmingDecisionTrueValue)
	} else {
		cookie.SetValue(dimmingDecisionFalseValue)
	}
	cookie.SetExpire(time.Now().Add(cookieDimmingDefaultExpiry))

	return cookie
}
