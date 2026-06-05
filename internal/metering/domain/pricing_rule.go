package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// TimeModifier adjusts the base cost during specific hours and/or days.
type TimeModifier struct {
	Hours      string   `json:"hours"`      // e.g. "09:00-17:00"
	Days       []string `json:"days"`       // e.g. ["mon","tue","wed","thu","fri"]
	Multiplier float64  `json:"multiplier"` // e.g. 1.5
}

// PricingRule defines the cost for an endpoint, including time-based modifiers.
type PricingRule struct {
	BaseCost      int64          `json:"base_cost"`
	TimeModifiers []TimeModifier `json:"time_modifiers,omitempty"`
}

// CostFor calculates the final cost for a request at the given time.
// It applies the first matching time modifier, or falls back to base_cost.
// The result is rounded to int64 (math.Round).
func (r PricingRule) CostFor(t time.Time) (finalCost int64, multiplier float64) {
	for i := range r.TimeModifiers {
		mod := &r.TimeModifiers[i]
		if mod.matches(t) {
			multiplier = mod.Multiplier
			finalCost = int64(math.Round(float64(r.BaseCost) * multiplier))
			return finalCost, multiplier
		}
	}
	return r.BaseCost, 1.0
}

// matches reports whether the current time falls within this modifier's window.
func (m *TimeModifier) matches(t time.Time) bool {
	if len(m.Days) > 0 && !m.dayMatches(t) {
		return false
	}
	if m.Hours == "" {
		return true
	}
	return m.hourMatches(t)
}

func (m *TimeModifier) dayMatches(t time.Time) bool {
	day := strings.ToLower(t.Weekday().String()[:3])
	for _, d := range m.Days {
		if strings.EqualFold(d, day) {
			return true
		}
	}
	return false
}

func (m *TimeModifier) hourMatches(t time.Time) bool {
	start, end, err := parseHourRange(m.Hours)
	if err != nil {
		return false
	}
	h, min := t.Hour(), t.Minute()
	minutes := h*60 + min
	return minutes >= start && minutes < end
}

// parseHourRange parses "HH:MM-HH:MM" into start and end minutes.
func parseHourRange(s string) (start, end int, err error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid hour range: %q", s)
	}
	start, err = parseHHMM(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end, err = parseHHMM(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

func parseHHMM(s string) (int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time: %q", s)
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, err
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, err
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("time out of range: %q", s)
	}
	return h*60 + m, nil
}
