package profiling

import (
	"github.com/valyala/fasthttp"
	"log"
	"math/rand"
	"time"
)

const priorityKey = "PRIORITY"
const priorityUnknownValue = "unknown"
const priorityLowValue = "low"
const priorityHighValue = "high"
const cookieUnknownDefaultExpiry = 2 * time.Minute
const cookiePriorityDefaultExpiry = 2 * time.Hour

const dimmingDecisionKey = "DIMMING_DECISION"
const dimmingDecisionTrueValue = "1"
const dimmingDecisionFalseValue = "0"
const cookieDimmingDefaultExpiry = 1 * time.Minute

const lowPriorityDimmingProbability = 0.9
const highPriorityDimmingProbability = 0.1

type Profiler struct {
	Priorities PriorityFetcher
	Requests   RequestWriter
}

func RequestHasPriorityCookie(request *fasthttp.Request) bool {
	return string(request.Header.Cookie(priorityKey)) == priorityLowValue ||
		string(request.Header.Cookie(priorityKey)) == priorityHighValue
}

func RequestHasUnknownCookie(request *fasthttp.Request) bool {
	return string(request.Header.Cookie(priorityKey)) == priorityUnknownValue
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

func SampleDimmingForPriorityCookie(request *fasthttp.Request) bool {
	if string(request.Header.Cookie(priorityKey)) == priorityLowValue {
		return rand.Float64() < lowPriorityDimmingProbability
	} else if string(request.Header.Cookie(priorityKey)) == priorityHighValue {
		return rand.Float64() < highPriorityDimmingProbability
	} else {
		log.Printf("unexpected priority cookie value during SampleDimmingForPriorityCookie: %s", string(request.Header.Cookie(priorityKey)))
		return false
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
