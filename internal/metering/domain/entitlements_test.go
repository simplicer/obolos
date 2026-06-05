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

func TestGrant_ResetMonthly(t *testing.T) {
	g := Grant{Mode: GrantReset, Amount: 1000, Anchor: "2026-05"}
	if !g.IsDue(fri10) {
		t.Fatal("reset grant from a prior month should be due")
	}
	nb, anchor := g.Applied(50, fri10)
	if nb != 1000 || anchor != "2026-06" {
		t.Errorf("reset: got %d/%s, want 1000/2026-06", nb, anchor)
	}
	// Same month → not due.
	if (Grant{Mode: GrantReset, Amount: 1000, Anchor: "2026-06"}).IsDue(fri10) {
		t.Error("reset grant in the same month must not be due")
	}
}

func TestGrant_RolloverWithCap(t *testing.T) {
	capped := Grant{Mode: GrantRollover, Amount: 1000, Cap: 1500, Anchor: "2026-05"}
	if nb, _ := capped.Applied(800, fri10); nb != 1500 {
		t.Errorf("rollover capped: got %d, want 1500", nb)
	}
	uncapped := Grant{Mode: GrantRollover, Amount: 1000, Anchor: "2026-05"}
	if nb, _ := uncapped.Applied(800, fri10); nb != 1800 {
		t.Errorf("rollover uncapped: got %d, want 1800", nb)
	}
}

func TestGrant_Rolling30Days(t *testing.T) {
	if !(Grant{Mode: GrantRolling, Amount: 500}).IsDue(fri10) {
		t.Error("rolling grant with empty anchor should be due")
	}
	recent := Grant{Mode: GrantRolling, Amount: 500, Anchor: fri10.Add(-10 * 24 * time.Hour).Format(time.RFC3339)}
	if recent.IsDue(fri10) {
		t.Error("rolling grant 10 days old must not be due")
	}
	old := Grant{Mode: GrantRolling, Amount: 500, Anchor: fri10.Add(-31 * 24 * time.Hour).Format(time.RFC3339)}
	if !old.IsDue(fri10) {
		t.Error("rolling grant 31 days old should be due")
	}
}

func TestEntitlements_Validate(t *testing.T) {
	valid := Entitlements{
		Windows:         []TimeWindow{{Hours: "09:00-17:00", Days: []string{"mon", "fri"}, Action: WindowMultiply, Factor: 1.5}},
		Grant:           Grant{Mode: GrantRollover, Amount: 1000, Cap: 5000},
		RateLimitPerMin: 600,
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid entitlements rejected: %v", err)
	}

	bad := []Entitlements{
		{Windows: []TimeWindow{{Action: "bogus"}}},
		{Windows: []TimeWindow{{Action: WindowMultiply, Factor: -1}}},
		{Windows: []TimeWindow{{Hours: "9-5", Action: WindowFree}}},
		{Windows: []TimeWindow{{Days: []string{"funday"}, Action: WindowFree}}},
		{Grant: Grant{Mode: "weekly", Amount: 1}},
		{Grant: Grant{Mode: GrantReset, Amount: -100}},
		{Grant: Grant{Mode: GrantRollover, Cap: -1}},
		{RateLimitPerMin: -5},
	}
	for i, e := range bad {
		if err := e.Validate(); err == nil {
			t.Errorf("case %d: expected validation error, got nil", i)
		}
	}
}

func TestEntitlements_NegativeFactorClampedToZero(t *testing.T) {
	// Defence in depth: even if a negative factor slipped past validation, the
	// adjusted cost must never go negative (which would credit the user).
	e := Entitlements{Windows: []TimeWindow{{Action: WindowMultiply, Factor: -2}}}
	cost, _, _ := e.ApplyWindowToCost(10, fri10)
	if cost != 0 {
		t.Errorf("negative factor: got cost %d, want 0", cost)
	}
}

func TestGrant_FloorNeverReducesBalance(t *testing.T) {
	// A reset grant of 1000 must NOT wipe a balance of 5000 (purchased credits).
	reset := Grant{Mode: GrantReset, Amount: 1000, Anchor: "2026-05"}
	if nb, _ := reset.Applied(5000, fri10); nb != 5000 {
		t.Errorf("reset floor: got %d, want 5000 (must not reduce)", nb)
	}
	// But it tops a low balance up to the floor.
	if nb, _ := reset.Applied(200, fri10); nb != 1000 {
		t.Errorf("reset floor: got %d, want 1000", nb)
	}
	rolling := Grant{Mode: GrantRolling, Amount: 500}
	if nb, _ := rolling.Applied(5000, fri10); nb != 5000 {
		t.Errorf("rolling floor: got %d, want 5000 (must not reduce)", nb)
	}
}

func TestGrant_NoneNeverDue(t *testing.T) {
	if (Grant{}).IsDue(fri10) {
		t.Error("empty grant must never be due")
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
