package domain

import "testing"

func TestNewBalance(t *testing.T) {
	b := NewBalance(100)
	if b.Value() != 100 {
		t.Errorf("expected 100, got %d", b.Value())
	}
}

func TestBalanceCanDebit(t *testing.T) {
	tests := []struct {
		name    string
		balance int64
		cost    int64
		want    bool
	}{
		{"exact balance", 100, 100, true},
		{"sufficient balance", 200, 100, true},
		{"insufficient balance", 50, 100, false},
		{"zero balance", 0, 1, false},
		{"zero cost", 100, 0, true},
		{"negative balance", -10, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBalance(tt.balance)
			if got := b.CanDebit(tt.cost); got != tt.want {
				t.Errorf("CanDebit(%d) = %v, want %v", tt.cost, got, tt.want)
			}
		})
	}
}

func TestBalanceDebit(t *testing.T) {
	b := NewBalance(100)
	result := b.Debit(30)
	if result.Value() != 70 {
		t.Errorf("expected 70, got %d", result.Value())
	}
}

func TestBalanceCredit(t *testing.T) {
	b := NewBalance(100)
	result := b.Credit(50)
	if result.Value() != 150 {
		t.Errorf("expected 150, got %d", result.Value())
	}
}

func TestBalanceIsNegative(t *testing.T) {
	tests := []struct {
		name    string
		balance int64
		want    bool
	}{
		{"positive", 100, false},
		{"zero", 0, false},
		{"negative", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBalance(tt.balance)
			if got := b.IsNegative(); got != tt.want {
				t.Errorf("IsNegative() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBalanceIsZero(t *testing.T) {
	if !NewBalance(0).IsZero() {
		t.Error("zero balance should be zero")
	}
	if NewBalance(1).IsZero() {
		t.Error("positive balance should not be zero")
	}
}

func TestBalanceDebitToNegative(t *testing.T) {
	// Hybrid plans allow going negative
	b := NewBalance(5)
	result := b.Debit(10)
	if result.Value() != -5 {
		t.Errorf("expected -5, got %d", result.Value())
	}
}

func TestBalanceImmutability(t *testing.T) {
	b := NewBalance(100)
	_ = b.Debit(50)
	_ = b.Credit(50)
	if b.Value() != 100 {
		t.Errorf("original balance should not be modified, got %d", b.Value())
	}
}
