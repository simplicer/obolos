package domain

import (
	"testing"
	"time"
)

// 2026-06-05 10:00 UTC is a Friday.
var fri10 = time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)

func TestEntitlements_NoWindowsNoEffect(t *testing.T) {
	e := Entitlements{}
	cost, blocked, matched := e.ApplyWindowToCost(10, fri10)
	if matched || blocked || cost != 10 {
		t.Errorf("no windows: got cost=%d blocked=%v matched=%v, want 10/false/false", cost, blocked, matched)
	}
}

func TestEntitlements_FreeWindow(t *testing.T) {
	e := Entitlements{Windows: []TimeWindow{{Hours: "09:00-17:00", Action: WindowFree}}}
	cost, blocked, matched := e.ApplyWindowToCost(10, fri10)
	if !matched || blocked || cost != 0 {
		t.Errorf("free window: got cost=%d blocked=%v matched=%v, want 0/false/true", cost, blocked, matched)
	}
}

func TestEntitlements_MultiplyWindow(t *testing.T) {
	e := Entitlements{Windows: []TimeWindow{{Hours: "09:00-17:00", Action: WindowMultiply, Factor: 1.5}}}
	cost, _, _ := e.ApplyWindowToCost(10, fri10)
	if cost != 15 {
		t.Errorf("multiply window: got %d, want 15", cost)
	}
}

func TestEntitlements_BlockWindow(t *testing.T) {
	e := Entitlements{Windows: []TimeWindow{{Hours: "09:00-17:00", Action: WindowBlock}}}
	_, blocked, matched := e.ApplyWindowToCost(10, fri10)
	if !matched || !blocked {
		t.Errorf("block window: got blocked=%v matched=%v, want true/true", blocked, matched)
	}
}

func TestEntitlements_OutsideHoursNoMatch(t *testing.T) {
	e := Entitlements{Windows: []TimeWindow{{Hours: "00:00-06:00", Action: WindowFree}}}
	cost, _, matched := e.ApplyWindowToCost(10, fri10)
	if matched || cost != 10 {
		t.Errorf("outside hours: got cost=%d matched=%v, want 10/false", cost, matched)
	}
}

func TestEntitlements_DayFilter(t *testing.T) {
	onFri := Entitlements{Windows: []TimeWindow{{Days: []string{"fri"}, Action: WindowBlock}}}
	if _, blocked, _ := onFri.ApplyWindowToCost(10, fri10); !blocked {
		t.Error("expected block on friday")
	}
	onMon := Entitlements{Windows: []TimeWindow{{Days: []string{"mon"}, Action: WindowBlock}}}
	if _, _, matched := onMon.ApplyWindowToCost(10, fri10); matched {
		t.Error("monday window must not match a friday")
	}
}

func TestEntitlements_FirstMatchWins(t *testing.T) {
	e := Entitlements{Windows: []TimeWindow{
		{Hours: "09:00-17:00", Action: WindowFree},
		{Hours: "09:00-17:00", Action: WindowBlock},
	}}
	cost, blocked, _ := e.ApplyWindowToCost(10, fri10)
	if blocked || cost != 0 {
		t.Errorf("first match should win (free): got cost=%d blocked=%v", cost, blocked)
	}
}
