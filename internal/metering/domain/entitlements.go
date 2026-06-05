package domain

import (
	"errors"
	"math"
	"strings"
	"time"
)

var (
	errInvalidHourRange    = errors.New("invalid hour range")
	errInvalidEntitlements = errors.New("invalid entitlements")
)

// ErrInvalidEntitlements is returned by Validate for malformed entitlement rules.
var ErrInvalidEntitlements = errInvalidEntitlements

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

// GrantMode selects how a periodic credit grant renews the balance.
type GrantMode string

const (
	// GrantReset: at each calendar-month boundary (UTC) the balance is topped up
	// to at least Amount (a monthly floor). It never reduces a higher balance,
	// so purchased credits are preserved.
	GrantReset GrantMode = "reset"
	// GrantRollover: at each calendar-month boundary Amount is added to the
	// balance, capped at Cap (0 = uncapped); unused credits roll over.
	GrantRollover GrantMode = "rollover"
	// GrantRolling: every 30 days from the last grant the balance is topped up
	// to at least Amount (a rolling floor); it never reduces a higher balance.
	GrantRolling GrantMode = "rolling"
)

// Grant describes a periodic credit grant. Mode == "" means no grant.
// Anchor is the marker of the last applied period ("YYYY-MM" for reset/rollover,
// an RFC3339 timestamp for rolling); it is maintained by the store.
type Grant struct {
	Mode   GrantMode `json:"mode,omitempty"`
	Amount int64     `json:"amount,omitempty"`
	Cap    int64     `json:"cap,omitempty"`
	Anchor string    `json:"anchor,omitempty"`
}

// IsDue reports whether a grant should be applied at time t given the anchor.
func (g Grant) IsDue(t time.Time) bool {
	switch g.Mode {
	case GrantReset, GrantRollover:
		return g.Anchor != t.UTC().Format("2006-01")
	case GrantRolling:
		if g.Anchor == "" {
			return true
		}
		at, err := time.Parse(time.RFC3339, g.Anchor)
		if err != nil {
			return true
		}
		return t.UTC().Sub(at) >= 30*24*time.Hour
	default:
		return false
	}
}

// Applied returns the new balance and the new anchor after applying the grant
// to the given balance at time t.
func (g Grant) Applied(balance int64, t time.Time) (newBalance int64, newAnchor string) {
	switch g.Mode {
	case GrantRollover:
		newBalance = balance + g.Amount
		if g.Cap > 0 && newBalance > g.Cap {
			newBalance = g.Cap
		}
	default: // reset, rolling: top up to the Amount floor, never reduce
		newBalance = balance
		if g.Amount > newBalance {
			newBalance = g.Amount
		}
	}
	if g.Mode == GrantRolling {
		return newBalance, t.UTC().Format(time.RFC3339)
	}
	return newBalance, t.UTC().Format("2006-01")
}

// Entitlements holds a user's credit-usage rules beyond the bare plan type.
// It is stored per user as a JSON document; the zero value means "no rules".
type Entitlements struct {
	Windows []TimeWindow `json:"windows,omitempty"`
	Grant   Grant        `json:"grant,omitempty"`
	// RateLimitPerMin caps charges per rolling 60s for the user (0 = unlimited).
	RateLimitPerMin int `json:"rate_limit_per_min,omitempty"`
}

// Validate checks that the entitlement rules are well-formed. It rejects the
// configurations that would otherwise misbehave silently or, worse, credit a
// user (e.g. a negative multiply factor) or push a balance negative.
func (e Entitlements) Validate() error {
	for i := range e.Windows {
		w := &e.Windows[i]
		switch w.Action {
		case WindowFree, WindowBlock:
		case WindowMultiply:
			if w.Factor < 0 {
				return errInvalidEntitlements
			}
		default:
			return errInvalidEntitlements
		}
		if w.Hours != "" {
			if _, _, err := parseHourRange(w.Hours); err != nil {
				return errInvalidEntitlements
			}
		}
		for _, d := range w.Days {
			if !validDay(d) {
				return errInvalidEntitlements
			}
		}
	}
	switch e.Grant.Mode {
	case "", GrantReset, GrantRollover, GrantRolling:
	default:
		return errInvalidEntitlements
	}
	if e.Grant.Amount < 0 || e.Grant.Cap < 0 ||
		e.Grant.Amount > MaxCredits || e.Grant.Cap > MaxCredits {
		return errInvalidEntitlements
	}
	if e.RateLimitPerMin < 0 {
		return errInvalidEntitlements
	}
	return nil
}

func validDay(d string) bool {
	switch strings.ToLower(d) {
	case "mon", "tue", "wed", "thu", "fri", "sat", "sun":
		return true
	default:
		return false
	}
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
		adjusted := int64(math.Round(float64(cost) * factor))
		if adjusted < 0 {
			adjusted = 0
		}
		return adjusted, false, true
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
