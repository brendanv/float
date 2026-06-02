package ui

import "testing"

func TestFormatTemplateTags(t *testing.T) {
	t.Parallel()

	got := formatTemplateTags(map[string]string{
		"method":  "ach",
		"cadence": "monthly",
		"flag":    "",
	})
	want := "cadence=monthly flag method=ach"
	if got != want {
		t.Fatalf("formatTemplateTags() = %q, want %q", got, want)
	}
}
