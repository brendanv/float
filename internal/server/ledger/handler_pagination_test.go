package ledger

import "testing"

func TestPaginate(t *testing.T) {
	tests := []struct {
		name      string
		items     []int
		offset    int32
		limit     int32
		wantItems []int
		wantTotal int32
		wantNext  bool
	}{
		{name: "no pagination", items: []int{1, 2, 3}, wantItems: []int{1, 2, 3}, wantTotal: 3},
		{name: "offset only", items: []int{1, 2, 3}, offset: 1, wantItems: []int{2, 3}, wantTotal: 3},
		{name: "limit only", items: []int{1, 2, 3}, limit: 2, wantItems: []int{1, 2}, wantTotal: 3, wantNext: true},
		{name: "offset and limit", items: []int{1, 2, 3, 4}, offset: 1, limit: 2, wantItems: []int{2, 3}, wantTotal: 4, wantNext: true},
		{name: "offset past end", items: []int{1, 2, 3}, offset: 10, wantItems: nil, wantTotal: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotItems, gotTotal, gotNext := paginate(tc.items, tc.offset, tc.limit)
			if gotTotal != tc.wantTotal {
				t.Fatalf("total: got %d, want %d", gotTotal, tc.wantTotal)
			}
			if gotNext != tc.wantNext {
				t.Fatalf("hasNext: got %v, want %v", gotNext, tc.wantNext)
			}
			if len(gotItems) != len(tc.wantItems) {
				t.Fatalf("len(items): got %d, want %d", len(gotItems), len(tc.wantItems))
			}
			for i := range gotItems {
				if gotItems[i] != tc.wantItems[i] {
					t.Fatalf("items[%d]: got %d, want %d", i, gotItems[i], tc.wantItems[i])
				}
			}
		})
	}
}
