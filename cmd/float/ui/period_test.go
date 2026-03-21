package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPeriodSelector_Query(t *testing.T) {
	tests := []struct {
		name  string
		year  int
		month time.Month
		want  string
	}{
		{"january", 2026, time.January, "date:2026-01"},
		{"march", 2026, time.March, "date:2026-03"},
		{"october", 2026, time.October, "date:2026-10"},
		{"december", 2025, time.December, "date:2025-12"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := PeriodSelector{year: tc.year, month: tc.month}
			if got := p.Query(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPeriodSelector_NavigateForward(t *testing.T) {
	tests := []struct {
		name        string
		startYear   int
		startMonth  time.Month
		wantYear    int
		wantMonth   time.Month
	}{
		{"march to april", 2026, time.March, 2026, time.April},
		{"november to december", 2026, time.November, 2026, time.December},
		{"december to january rollover", 2025, time.December, 2026, time.January},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := PeriodSelector{year: tc.startYear, month: tc.startMonth}
			p2, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
			if p2.month != tc.wantMonth || p2.year != tc.wantYear {
				t.Errorf("got %v %d, want %v %d", p2.month, p2.year, tc.wantMonth, tc.wantYear)
			}
			if cmd == nil {
				t.Error("expected non-nil cmd on period change")
			}
			msg := cmd()
			if _, ok := msg.(PeriodChangedMsg); !ok {
				t.Errorf("expected PeriodChangedMsg, got %T", msg)
			}
		})
	}
}

func TestPeriodSelector_NavigateBackward(t *testing.T) {
	tests := []struct {
		name       string
		startYear  int
		startMonth time.Month
		wantYear   int
		wantMonth  time.Month
	}{
		{"march to february", 2026, time.March, 2026, time.February},
		{"february to january", 2026, time.February, 2026, time.January},
		{"january to december rollover", 2026, time.January, 2025, time.December},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := PeriodSelector{year: tc.startYear, month: tc.startMonth}
			p2, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
			if p2.month != tc.wantMonth || p2.year != tc.wantYear {
				t.Errorf("got %v %d, want %v %d", p2.month, p2.year, tc.wantMonth, tc.wantYear)
			}
			if cmd == nil {
				t.Error("expected non-nil cmd on period change")
			}
		})
	}
}

func TestPeriodSelector_NoChangedMsgOnOtherKeys(t *testing.T) {
	p := PeriodSelector{year: 2026, month: time.March}
	tests := []struct {
		key string
	}{
		{"j"}, {"k"}, {"q"}, {"tab"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
			if cmd != nil {
				t.Errorf("expected nil cmd for key %q, got non-nil", tc.key)
			}
		})
	}
}

func TestPeriodSelector_View_ContainsMonthAndYear(t *testing.T) {
	p := PeriodSelector{year: 2026, month: time.March, width: 40}
	view := p.View()
	if len(view) == 0 {
		t.Fatal("expected non-empty view")
	}
	// Should contain the month name and year
	for _, s := range []string{"March", "2026", "<<<", ">>>"} {
		found := false
		for i := 0; i+len(s) <= len(view); i++ {
			if view[i:i+len(s)] == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("view %q does not contain %q", view, s)
		}
	}
}
