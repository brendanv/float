package journal

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func lotsDataDir(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "hledger", "testdata", "lots.journal"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), src, 0o644); err != nil {
		t.Fatalf("write main.journal: %v", err)
	}
	return dir
}

func TestPostingAmountStringCostBasis(t *testing.T) {
	tests := []struct {
		name string
		p    PostingInput
		want string
	}{
		{
			name: "nil cost basis",
			p:    PostingInput{Commodity: "AAPL", Quantity: "10"},
			want: "10 AAPL",
		},
		{
			name: "empty cost basis",
			p:    PostingInput{Commodity: "AAPL", Quantity: "10", CostBasis: &CostBasisInput{}},
			want: "10 AAPL",
		},
		{
			name: "date only",
			p:    PostingInput{Commodity: "AAPL", Quantity: "-5", CostBasis: &CostBasisInput{Date: "2026-01-15"}},
			want: "-5 AAPL {2026-01-15}",
		},
		{
			name: "cost only",
			p:    PostingInput{Commodity: "AAPL", Quantity: "10", CostBasis: &CostBasisInput{Commodity: "$", Quantity: "50"}},
			want: "10 AAPL {50 $}",
		},
		{
			name: "date label cost",
			p: PostingInput{Commodity: "AAPL", Quantity: "4", CostBasis: &CostBasisInput{
				Date: "2026-01-16", Label: "mine", Commodity: "$", Quantity: "40",
			}},
			want: `4 AAPL {2026-01-16, "mine", 40 $}`,
		},
		{
			name: "label and cost",
			p: PostingInput{Commodity: "AAPL", Quantity: "4", CostBasis: &CostBasisInput{
				Label: "mine", Commodity: "$", Quantity: "40",
			}},
			want: `4 AAPL {"mine", 40 $}`,
		},
		{
			name: "combined with unit cost",
			p: PostingInput{
				Commodity: "AAPL", Quantity: "-5",
				Cost:      &CostInput{Commodity: "$", Quantity: "70"},
				CostBasis: &CostBasisInput{Date: "2026-01-15", Commodity: "$", Quantity: "50"},
			},
			want: "-5 AAPL {2026-01-15, 50 $} @ 70 $",
		},
		{
			name: "combined with total cost",
			p: PostingInput{
				Commodity: "AAPL", Quantity: "-5",
				Cost:      &CostInput{Commodity: "$", Quantity: "350", IsTotal: true},
				CostBasis: &CostBasisInput{Date: "2026-01-15"},
			},
			want: "-5 AAPL {2026-01-15} @@ 350 $",
		},
		{
			name: "auto-balance posting ignores cost basis",
			p:    PostingInput{CostBasis: &CostBasisInput{Date: "2026-01-15"}},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := postingAmountString(tc.p); got != tc.want {
				t.Errorf("postingAmountString = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLotRoundTrip(t *testing.T) {
	const (
		bigCostFID  = "aa000004"
		labelledFID = "aa000002"
		disposalFID = "aa000005"
		totalFID    = "aa000006"
	)

	tests := []struct {
		name     string
		fid      string
		extra    string
		mutate   func(t *testing.T, dir, fid string)
		wantText []string
	}{
		{
			name: "modify tags on disposal",
			fid:  disposalFID,
			mutate: func(t *testing.T, dir, fid string) {
				client := mustHledgerClient(t, dir)
				if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"k": "v"}); err != nil {
					t.Fatalf("ModifyTags: %v", err)
				}
			},
			wantText: []string{"{2026-01-15", "k:v"},
		},
		{
			name: "status on disposal",
			fid:  disposalFID,
			mutate: func(t *testing.T, dir, fid string) {
				client := mustHledgerClient(t, dir)
				if err := UpdateTransactionStatus(t.Context(), client, dir, fid, "Cleared"); err != nil {
					t.Fatalf("UpdateTransactionStatus: %v", err)
				}
			},
			wantText: []string{"{2026-01-15"},
		},
		{
			name: "date on disposal",
			fid:  disposalFID,
			mutate: func(t *testing.T, dir, fid string) {
				client := mustHledgerClient(t, dir)
				if _, err := UpdateTransactionDate(t.Context(), client, dir, fid, "2026-02-10"); err != nil {
					t.Fatalf("UpdateTransactionDate: %v", err)
				}
			},
			wantText: []string{"{2026-01-15", "2026-02-10"},
		},
		{
			name: "modify tags on labelled acquire",
			fid:  labelledFID,
			mutate: func(t *testing.T, dir, fid string) {
				client := mustHledgerClient(t, dir)
				if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"k": "v"}); err != nil {
					t.Fatalf("ModifyTags: %v", err)
				}
			},
			wantText: []string{`"mine"`, "2026-01-16"},
		},
		{
			name: "modify tags on fractional cost acquire",
			fid:  bigCostFID,
			mutate: func(t *testing.T, dir, fid string) {
				client := mustHledgerClient(t, dir)
				if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"k": "v"}); err != nil {
					t.Fatalf("ModifyTags: %v", err)
				}
			},
			wantText: []string{"{1234.567 $}"},
		},
		{
			name:  "modify tags on total cost acquire",
			fid:   totalFID,
			extra: "\n2026-01-26 (aa000006) Buy AAPL total cost\n    assets:brokerage    3 AAPL @@ $100\n    assets:cash\n",
			mutate: func(t *testing.T, dir, fid string) {
				client := mustHledgerClient(t, dir)
				if err := ModifyTags(t.Context(), client, dir, fid, map[string]string{"k": "v"}); err != nil {
					t.Fatalf("ModifyTags: %v", err)
				}
			},
			wantText: []string{"@@ 100 $"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := lotsDataDir(t)
			if tc.extra != "" {
				f, err := os.OpenFile(filepath.Join(dir, "main.journal"), os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatalf("open main.journal: %v", err)
				}
				_, err = f.WriteString(tc.extra)
				if cerr := f.Close(); err == nil {
					err = cerr
				}
				if err != nil {
					t.Fatalf("append fixture: %v", err)
				}
			}
			before, err := mustHledgerClient(t, dir).Lots(t.Context())
			if err != nil {
				t.Fatalf("Lots before: %v", err)
			}
			tc.mutate(t, dir, tc.fid)

			client := mustHledgerClient(t, dir)
			if err := client.Check(t.Context()); err != nil {
				t.Fatalf("hledger check: %v", err)
			}

			after, err := client.Lots(t.Context())
			if err != nil {
				t.Fatalf("Lots after: %v", err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Errorf("lots changed:\nbefore: %+v\nafter:  %+v", before, after)
			}

			files, err := filepath.Glob(filepath.Join(dir, "*", "*.journal"))
			if err != nil {
				t.Fatalf("glob: %v", err)
			}
			files = append(files, filepath.Join(dir, "main.journal"))
			var sb strings.Builder
			for _, f := range files {
				raw, err := os.ReadFile(f)
				if err != nil {
					t.Fatalf("read %s: %v", f, err)
				}
				sb.Write(raw)
			}
			text := sb.String()
			for _, w := range tc.wantText {
				if !strings.Contains(text, w) {
					t.Errorf("journal missing %q:\n%s", w, text)
				}
			}
			if strings.Contains(text, "revenues:gain") {
				t.Errorf("explicit gain posting written:\n%s", text)
			}
		})
	}
}
