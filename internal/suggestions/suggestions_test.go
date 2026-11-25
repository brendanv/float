package suggestions

import (
	"os"
	"reflect"
	"testing"
)

func TestLoadMissingIsEmpty(t *testing.T) {
	s, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s) != 0 {
		t.Errorf("Load = %v, want empty", s)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Store{
		"aa000001": {Account: "expenses:food", Confidence: "high", Reasoning: "r", Model: "m", CreatedAt: "2026-01-01T00:00:00Z"},
		"aa000002": {CreatedAt: "2026-01-01T00:00:00Z"},
	}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %v, want %v", got, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != FileName {
		t.Errorf("dir entries = %v, want only %s", entries, FileName)
	}
}

func TestLoadCorruptFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(FilePath(dir), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("Load of corrupt file succeeded")
	}
}
