package txlock_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/brendanv/float/internal/hledger"
	"github.com/brendanv/float/internal/txlock"
)

// setupDataDir creates a minimal valid data directory:
//   - accounts.journal with a few account declarations
//   - main.journal that includes accounts.journal
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

// mustTxLock creates a TxLock with a real hledger client pointed at dataDir/main.journal.
// Skips the test if hledger is unavailable.
func mustTxLock(t *testing.T, dataDir string) *txlock.TxLock {
	t.Helper()
	client, err := hledger.New("hledger", filepath.Join(dataDir, "main.journal"))
	if err != nil {
		t.Skipf("hledger unavailable: %v", err)
	}
	return txlock.New(dataDir, client)
}

// addMonthFile is a helper fn for use inside Do: writes content to YYYY/MM.journal
// and appends an include directive to main.journal.
func addMonthFile(dir, relPath, content string) func() error {
	return func() error {
		abs := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
			return err
		}
		mainPath := filepath.Join(dir, "main.journal")
		f, err := os.OpenFile(mainPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = fmt.Fprintln(f, "include "+relPath)
		return err
	}
}

func TestTxLock_Do(t *testing.T) {
	// A valid balanced transaction (hledger check passes).
	validTx := "2026-01-15 AMAZON\n    expenses:food  $10.00\n    assets:checking  $-10.00\n\n"

	// An unbalanced transaction (hledger check fails).
	invalidTx := "2026-01-15 BROKEN\n    expenses:food  $10.00\n    assets:checking  $5.00\n\n"

	tests := []struct {
		name           string
		fn             func(dir string) func() error
		wantErr        bool
		wantCheckErr   bool
		wantGeneration uint64
		check          func(t *testing.T, dir string)
	}{
		{
			name: "valid write persists and bumps generation",
			fn:   func(dir string) func() error { return addMonthFile(dir, "2026/01.journal", validTx) },
			wantGeneration: 1,
			check: func(t *testing.T, dir string) {
				data, err := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(data), "AMAZON") {
					t.Error("transaction not found in month file")
				}
				main, _ := os.ReadFile(filepath.Join(dir, "main.journal"))
				if !strings.Contains(string(main), "include 2026/01.journal") {
					t.Error("main.journal missing include directive")
				}
			},
		},
		{
			name:         "invalid write reverts files and returns check error",
			fn:           func(dir string) func() error { return addMonthFile(dir, "2026/01.journal", invalidTx) },
			wantErr:      true,
			wantCheckErr: true,
			wantGeneration: 0,
			check: func(t *testing.T, dir string) {
				// New month file should have been deleted
				if _, err := os.Stat(filepath.Join(dir, "2026/01.journal")); !os.IsNotExist(err) {
					t.Error("2026/01.journal should not exist after revert")
				}
				// main.journal should be restored to original (no include for month)
				main, _ := os.ReadFile(filepath.Join(dir, "main.journal"))
				if strings.Contains(string(main), "2026/01.journal") {
					t.Errorf("main.journal should not contain month include after revert:\n%s", main)
				}
			},
		},
		{
			name: "fn error returned directly without check or revert",
			fn: func(dir string) func() error {
				return func() error { return errors.New("fn failed") }
			},
			wantErr:        true,
			wantGeneration: 0,
		},
		{
			name: "second valid write bumps generation to 2",
			fn:   func(dir string) func() error { return addMonthFile(dir, "2026/01.journal", validTx) },
			wantGeneration: 2, // this test calls Do twice (see below)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupDataDir(t)
			l := mustTxLock(t, dir)

			// For the "second write" test, prime with a first successful write.
			if tt.wantGeneration == 2 {
				first := addMonthFile(dir, "2026/01.journal", validTx)
				if err := l.Do(t.Context(), first); err != nil {
					t.Fatalf("first Do() failed: %v", err)
				}
				// Now write to a different month so main.journal gets another include.
				second := addMonthFile(dir, "2026/02.journal", "2026-02-01 PAYROLL\n    assets:checking  $3500.00\n    income:salary  $-3500.00\n\n")
				if err := l.Do(t.Context(), second); err != nil {
					t.Fatalf("second Do() failed: %v", err)
				}
				if got := l.Generation(); got != 2 {
					t.Errorf("Generation() = %d, want 2", got)
				}
				return
			}

			err := l.Do(t.Context(), tt.fn(dir))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Do() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantCheckErr {
				var checkErr *hledger.CheckError
				if !errors.As(err, &checkErr) {
					t.Errorf("expected *hledger.CheckError, got %T: %v", err, err)
				}
			}
			if got := l.Generation(); got != tt.wantGeneration {
				t.Errorf("Generation() = %d, want %d", got, tt.wantGeneration)
			}
			if tt.check != nil {
				tt.check(t, dir)
			}
		})
	}
}

func TestTxLock_Do_ExistingFileReverted(t *testing.T) {
	// Verify that an existing journal file that fn modifies is restored on check failure.
	dir := setupDataDir(t)
	l := mustTxLock(t, dir)

	// Write a valid month file first.
	validTx := "2026-01-15 AMAZON\n    expenses:food  $10.00\n    assets:checking  $-10.00\n\n"
	if err := l.Do(t.Context(), addMonthFile(dir, "2026/01.journal", validTx)); err != nil {
		t.Fatalf("setup Do() failed: %v", err)
	}
	originalContent, _ := os.ReadFile(filepath.Join(dir, "2026/01.journal"))

	// Now attempt to append an invalid transaction to the existing month file.
	badAppend := func() error {
		f, err := os.OpenFile(filepath.Join(dir, "2026/01.journal"), os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = f.WriteString("2026-01-20 BROKEN\n    expenses:food  $10.00\n    assets:checking  $5.00\n\n")
		return err
	}
	if err := l.Do(t.Context(), badAppend); err == nil {
		t.Fatal("Do() with invalid append should have returned error")
	}

	// Existing file should be restored to its content after the first valid write.
	restored, _ := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
	if string(restored) != string(originalContent) {
		t.Errorf("existing file not restored after failed write\ngot:\n%s\nwant:\n%s", restored, originalContent)
	}
	// Generation should still be 1 (only the first successful write counted).
	if got := l.Generation(); got != 1 {
		t.Errorf("Generation() = %d, want 1", got)
	}
}

func TestTxLock_Do_Concurrent(t *testing.T) {
	// Two goroutines calling Do() concurrently must not corrupt the journal.
	dir := setupDataDir(t)
	l := mustTxLock(t, dir)

	var wg sync.WaitGroup
	errs := make([]error, 2)

	months := []string{"2026/01.journal", "2026/02.journal"}
	txns := []string{
		"2026-01-15 FIRST\n    expenses:food  $10.00\n    assets:checking  $-10.00\n\n",
		"2026-02-15 SECOND\n    expenses:food  $20.00\n    assets:checking  $-20.00\n\n",
	}

	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = l.Do(t.Context(), addMonthFile(dir, months[i], txns[i]))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Do() error = %v", i, err)
		}
	}
	if got := l.Generation(); got != 2 {
		t.Errorf("Generation() = %d, want 2", got)
	}
}
