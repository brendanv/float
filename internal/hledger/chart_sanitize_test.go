package hledger

import (
	"strings"
	"testing"
)

func TestSanitizeSVG(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		darkSelector string
		want         []string // substrings that must appear
		notWant      []string // substrings that must not appear
		wantTitle    string
		wantEmpty    bool
		wantErr      bool
	}{
		{
			name: "keeps allowlisted markup and extracts title",
			in: `<svg xmlns="http://www.w3.org/2000/svg" class="hledger-chart" viewBox="0 0 10 10" role="img">` +
				`<title>Income &amp; Expenses</title>` +
				`<g><title>expenses:food · 5 USD</title><path class="fill tx0" d="M 0 0 H 5"/></g></svg>`,
			want: []string{
				`<svg xmlns="http://www.w3.org/2000/svg" class="hledger-chart" viewBox="0 0 10 10" role="img">`,
				`<title>Income &amp; Expenses</title>`,
				`<path class="fill tx0" d="M 0 0 H 5"></path>`,
			},
			wantTitle: "Income & Expenses",
		},
		{
			name:      "no groups means empty",
			in:        `<svg xmlns="http://www.w3.org/2000/svg"><rect class="bg"/><text class="label">No data</text></svg>`,
			want:      []string{"No data"},
			wantEmpty: true,
		},
		{
			name: "drops scripts, foreign content, event handlers and external refs",
			in: `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" onload="alert(1)">` +
				`<script>alert(1)</script>` +
				`<foreignObject><div xmlns="http://www.w3.org/1999/xhtml">x</div></foreignObject>` +
				`<g><rect onclick="alert(2)" style="fill:red" xlink:href="https://evil" fill="url(https://evil/x)" width="1"/></g>` +
				`<!-- comment --></svg>`,
			notWant: []string{"script", "alert", "foreignObject", "<div", "style=", "evil", "href", "comment"},
			want:    []string{`<rect width="1"></rect>`},
		},
		{
			name: "escapes journal text",
			in: `<svg xmlns="http://www.w3.org/2000/svg"><g><title>expenses:&lt;script&gt;alert(1)&lt;/script&gt;</title>` +
				`<text>a &amp; &quot;b&quot;</text></g></svg>`,
			want:    []string{`<title>expenses:&lt;script&gt;alert(1)&lt;/script&gt;</title>`, `a &amp; "b"`},
			notWant: []string{"<script"},
		},
		{
			name: "namespaces ids and url references",
			in: `<svg xmlns="http://www.w3.org/2000/svg"><defs><clipPath id="clip_1"><path d="M0 0"/></clipPath>` +
				`<pattern id="hatch"/></defs><g clip-path="url(#clip_1)"><path fill="url(#hatch)"/></g></svg>`,
			want: []string{`id="pfx-clip_1"`, `clip-path="url(#pfx-clip_1)"`, `id="pfx-hatch"`, `fill="url(#pfx-hatch)"`},
		},
		{
			name: "rescopes dark mode styles",
			in: `<svg xmlns="http://www.w3.org/2000/svg"><style>.hledger-chart .bg{fill:#fff}` +
				"\n@media (prefers-color-scheme:dark){\n.hledger-chart .bg{fill:#000}\n.hledger-chart .s0{fill:#111}\n}\n.hledger-chart .x{fill:#222}</style></svg>",
			darkSelector: ".dark",
			want: []string{
				".hledger-chart .bg{fill:#fff}",
				":where(.dark) .hledger-chart .bg{fill:#000}",
				":where(.dark) .hledger-chart .s0{fill:#111}",
				"\n.hledger-chart .x{fill:#222}",
			},
			notWant:   []string{"@media", "prefers-color-scheme"},
			wantEmpty: true,
		},
		{
			name:      "keeps media query without dark selector",
			in:        `<svg xmlns="http://www.w3.org/2000/svg"><style>@media (prefers-color-scheme:dark){.hledger-chart .bg{fill:#000}}</style></svg>`,
			want:      []string{"@media (prefers-color-scheme:dark){.hledger-chart .bg{fill:#000}}"},
			notWant:   []string{":where"},
			wantEmpty: true,
		},
		{
			name:      "drops stylesheets that load resources",
			in:        `<svg xmlns="http://www.w3.org/2000/svg"><style>@import "https://evil/x.css";</style></svg>`,
			want:      []string{"<style></style>"},
			notWant:   []string{"evil"},
			wantEmpty: true,
		},
		{
			name:    "rejects non-svg root",
			in:      `<html><svg xmlns="http://www.w3.org/2000/svg"/></html>`,
			wantErr: true,
		},
		{
			name:    "rejects malformed xml",
			in:      `<svg xmlns="http://www.w3.org/2000/svg"><g></svg>`,
			wantErr: true,
		},
		{
			name:    "rejects missing root",
			in:      ``,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeSVG([]byte(tt.in), "pfx-", tt.darkSelector)
			if (err != nil) != tt.wantErr {
				t.Fatalf("sanitizeSVG() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			for _, s := range tt.want {
				if !strings.Contains(got.SVG, s) {
					t.Errorf("SVG missing %q\ngot: %s", s, got.SVG)
				}
			}
			for _, s := range tt.notWant {
				if strings.Contains(got.SVG, s) {
					t.Errorf("SVG unexpectedly contains %q\ngot: %s", s, got.SVG)
				}
			}
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Empty != tt.wantEmpty {
				t.Errorf("Empty = %v, want %v", got.Empty, tt.wantEmpty)
			}
		})
	}
}
