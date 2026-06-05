package domain

import (
	"errors"
	"math"
	"strings"
	"time"
)

var errInvalidHourRange = errors.New("invalid hour range")

// WindowAction is what a time window does to a charge that falls within it.
type WindowAction string

const (
	// WindowFree: the request passes but no credits are deducted.
	WindowFree WindowAction = "free"
	// WindowMultiply: the reported cost is multiplied by Factor.
	WindowMultiply WindowAction = "multiply"
	// WindowBlock: the request is denied for the duration of the window.
	WindowBlock WindowAction = "block"
)

// TimeWindow is a recurring time range (UTC) with an effect on the charge.
// An empty Days matches every day; an empty Hours matches the whole day.
type TimeWindow struct {
	Hours  string       `json:"hours,omitempty"` // "HH:MM-HH:MM", UTC
	Days   []string     `json:"days,omitempty"`  // ["mon","tue",...]
	Action WindowAction `json:"action"`
	Factor float64      `json:"factor,omitempty"` // used by WindowMultiply
}

// Entitlements holds a user's credit-usage rules beyond the bare plan type.
// It is stored per user as a JSON document; the zero value means "no rules".
type Entitlements struct {
	Windows []TimeWindow `json:"windows,omitempty"`
}

// WindowEffect returns the first matching window's action and factor at time t.
// matched is false when no window applies (charge proceeds normally).
func (e Entitlements) WindowEffect(t time.Time) (action WindowAction, factor float64, matched bool) {
	for i := range e.Windows {
		w := &e.Windows[i]
		if w.matches(t) {
			return w.Action, w.Factor, true
		}
	}
	return "", 0, false
}

// ApplyWindowToCost applies the matching window's effect to cost at time t.
// It returns the adjusted cost, whether the request is blocked, and whether a
// window matched at all.
func (e Entitlements) ApplyWindowToCost(cost int64, t time.Time) (adjusted int64, blocked bool, matched bool) {
	action, factor, ok := e.WindowEffect(t)
	if !ok {
		return cost, false, false
	}
	switch action {
	case WindowBlock:
		return cost, true, true
	case WindowFree:
		return 0, false, true
	case WindowMultiply:
		return int64(math.Round(float64(cost) * factor)), false, true
	default:
		return cost, false, true
	}
}

func (w *TimeWindow) matches(t time.Time) bool {
	if len(w.Days) > 0 && !w.dayMatches(t) {
		return false
	}
	if w.Hours == "" {
		return true
	}
	return w.hourMatches(t)
}

func (w *TimeWindow) dayMatches(t time.Time) bool {
	day := strings.ToLower(t.Weekday().String()[:3])
	for _, d := range w.Days {
		if strings.EqualFold(d, day) {
			return true
		}
	}
	return false
}

func (w *TimeWindow) hourMatches(t time.Time) bool {
	start, end, err := parseHourRange(w.Hours)
	if err != nil {
		return false
	}
	minutes := t.Hour()*60 + t.Minute()
	return minutes >= start && minutes < end
}

func parseHourRange(s string) (start, end int, err error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, errInvalidHourRange
	}
	if start, err = parseHHMM(parts[0]); err != nil {
		return 0, 0, err
	}
	if end, err = parseHHMM(parts[1]); err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func parseHHMM(s string) (int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, errInvalidHourRange
	}
	h, err1 := atoiBounded(parts[0], 0, 23)
	m, err2 := atoiBounded(parts[1], 0, 59)
	if err1 != nil {
		return 0, err1
	}
	if err2 != nil {
		return 0, err2
	}
	return h*60 + m, nil
}

func atoiBounded(s string, min, max int) (int, error) {
	n := 0
	if s == "" {
		return 0, errInvalidHourRange
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidHourRange
		}
		n = n*10 + int(c-'0')
	}
	if n < min || n > max {
		return 0, errInvalidHourRange
	}
	return n, nil
}
