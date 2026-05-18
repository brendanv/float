package migrate_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/migrate"
	"github.com/brendanv/float/internal/txlock"
)

func setupDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	accounts := "account assets:checking\naccount expenses:food\naccount income:salary\n"
	if err := os.WriteFile(filepath.Join(dir, "accounts.journal"), []byte(accounts), 0644); err != nil {
		t.Fatal(err)
	}
	main := "include accounts.journal\n"
	if err := os.WriteFile(filepath.Join(dir, "main.journal"), []byte(main), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustTxLock(t *testing.T, dataDir string) *txlock.TxLock {
	t.Helper()
	client, err := hledger.New("hledger", filepath.Join(dataDir, "main.journal"))
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	return txlock.New(dataDir, client)
}

func readApplied(t *testing.T, dataDir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dataDir, "migrations.json"))
	if err != nil {
		t.Fatalf("read migrations.json: %v", err)
	}
	var s struct {
		Applied []string `json:"applied"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse migrations.json: %v", err)
	}
	return s.Applied
}

func TestRunAll_NoMigrations(t *testing.T) {
	dir := setupDataDir(t)
	lock := mustTxLock(t, dir)

	if err := migrate.RunAll(t.Context(), nil, lock, nil, dir, nil); err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}

	// migrations.json should not be created when there's nothing to run.
	if _, err := os.Stat(filepath.Join(dir, "migrations.json")); !os.IsNotExist(err) {
		t.Error("migrations.json should not exist when no migrations ran")
	}
}

func TestRunAll_SingleMigration(t *testing.T) {
	dir := setupDataDir(t)
	lock := mustTxLock(t, dir)

	ran := false
	migrations := []migrate.Migration{
		{
			ID:          "0001_test",
			Description: "test migration",
			Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
				ran = true
				return nil
			},
		},
	}

	if err := migrate.RunAll(t.Context(), migrations, lock, nil, dir, nil); err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}

	if !ran {
		t.Error("migration Run was not called")
	}

	applied := readApplied(t, dir)
	if len(applied) != 1 || applied[0] != "0001_test" {
		t.Errorf("applied = %v, want [0001_test]", applied)
	}
}

func TestRunAll_AlreadyAppliedSkipped(t *testing.T) {
	dir := setupDataDir(t)
	lock := mustTxLock(t, dir)

	calls := 0
	migrations := []migrate.Migration{
		{
			ID:          "0001_test",
			Description: "test migration",
			Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
				calls++
				return nil
			},
		},
	}

	// First run — migration should execute.
	if err := migrate.RunAll(t.Context(), migrations, lock, nil, dir, nil); err != nil {
		t.Fatalf("first RunAll() error = %v", err)
	}

	// Second run — migration should be skipped.
	if err := migrate.RunAll(t.Context(), migrations, lock, nil, dir, nil); err != nil {
		t.Fatalf("second RunAll() error = %v", err)
	}

	if calls != 1 {
		t.Errorf("Run called %d times, want 1", calls)
	}
}

func TestRunAll_FailedMigrationNotRecorded(t *testing.T) {
	dir := setupDataDir(t)
	lock := mustTxLock(t, dir)

	migrations := []migrate.Migration{
		{
			ID:          "0001_failing",
			Description: "always fails",
			Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
				return errors.New("migration error")
			},
		},
	}

	err := migrate.RunAll(t.Context(), migrations, lock, nil, dir, nil)
	if err == nil {
		t.Fatal("RunAll() should have returned error")
	}

	// migrations.json should not be created since the migration failed.
	if _, statErr := os.Stat(filepath.Join(dir, "migrations.json")); !os.IsNotExist(statErr) {
		t.Error("migrations.json should not exist after failed migration")
	}
}

func TestRunAll_MultipleInOrder(t *testing.T) {
	dir := setupDataDir(t)
	lock := mustTxLock(t, dir)

	var order []string
	migrations := []migrate.Migration{
		{ID: "0001_a", Description: "first", Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
			order = append(order, "0001_a")
			return nil
		}},
		{ID: "0002_b", Description: "second", Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
			order = append(order, "0002_b")
			return nil
		}},
		{ID: "0003_c", Description: "third", Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
			order = append(order, "0003_c")
			return nil
		}},
	}

	if err := migrate.RunAll(t.Context(), migrations, lock, nil, dir, nil); err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}

	want := []string{"0001_a", "0002_b", "0003_c"}
	if len(order) != len(want) {
		t.Fatalf("execution order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want[i])
		}
	}

	applied := readApplied(t, dir)
	if len(applied) != 3 {
		t.Fatalf("applied = %v, want 3 entries", applied)
	}
	for i := range want {
		if applied[i] != want[i] {
			t.Errorf("applied[%d] = %q, want %q", i, applied[i], want[i])
		}
	}
}

func TestRunAll_PartialApply_SecondRunCompletesRest(t *testing.T) {
	dir := setupDataDir(t)
	lock := mustTxLock(t, dir)

	// First run: only migration 0001 (0002 fails).
	firstRun := []migrate.Migration{
		{ID: "0001_a", Description: "first", Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
			return nil
		}},
		{ID: "0002_b", Description: "second", Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
			return errors.New("fail")
		}},
	}
	if err := migrate.RunAll(t.Context(), firstRun, lock, nil, dir, nil); err == nil {
		t.Fatal("first RunAll() should have returned error")
	}

	applied := readApplied(t, dir)
	if len(applied) != 1 || applied[0] != "0001_a" {
		t.Errorf("after first run: applied = %v, want [0001_a]", applied)
	}

	// Second run: 0001 is skipped, 0002 now succeeds.
	calls := map[string]int{}
	secondRun := []migrate.Migration{
		{ID: "0001_a", Description: "first", Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
			calls["0001_a"]++
			return nil
		}},
		{ID: "0002_b", Description: "second", Run: func(ctx context.Context, dataDir string, hl *hledger.Client) error {
			calls["0002_b"]++
			return nil
		}},
	}
	if err := migrate.RunAll(t.Context(), secondRun, lock, nil, dir, nil); err != nil {
		t.Fatalf("second RunAll() error = %v", err)
	}

	if calls["0001_a"] != 0 {
		t.Error("0001_a should have been skipped on second run")
	}
	if calls["0002_b"] != 1 {
		t.Errorf("0002_b Run called %d times, want 1", calls["0002_b"])
	}

	applied = readApplied(t, dir)
	if len(applied) != 2 {
		t.Fatalf("after second run: applied = %v, want 2 entries", applied)
	}
}
