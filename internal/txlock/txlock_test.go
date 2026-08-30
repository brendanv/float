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
			// Failed writes bump the generation too: a concurrent read could
			// have cached the intermediate (reverted) state under the old
			// generation, so it must be invalidated.
			wantGeneration: 1,
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
			name: "fn error reverts partial writes and returns fn error",
			fn: func(dir string) func() error {
				return func() error {
					// Write one file successfully, then fail — simulates partial write.
					abs := filepath.Join(dir, "2026/01.journal")
					if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
						return err
					}
					if err := os.WriteFile(abs, []byte("partial"), 0644); err != nil {
						return err
					}
					return errors.New("fn failed mid-write")
				}
			},
			wantErr:        true,
			wantGeneration: 1, // failed writes also invalidate caches
			check: func(t *testing.T, dir string) {
				if _, err := os.Stat(filepath.Join(dir, "2026/01.journal")); !os.IsNotExist(err) {
					t.Error("partially written file should have been reverted")
				}
			},
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
				if err := l.Do(t.Context(), "test write", first); err != nil {
					t.Fatalf("first Do() failed: %v", err)
				}
				// Now write to a different month so main.journal gets another include.
				second := addMonthFile(dir, "2026/02.journal", "2026-02-01 PAYROLL\n    assets:checking  $3500.00\n    income:salary  $-3500.00\n\n")
				if err := l.Do(t.Context(), "test write", second); err != nil {
					t.Fatalf("second Do() failed: %v", err)
				}
				if got := l.Generation(); got != 2 {
					t.Errorf("Generation() = %d, want 2", got)
				}
				return
			}

			err := l.Do(t.Context(), "test write", tt.fn(dir))
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
	if err := l.Do(t.Context(), "test write", addMonthFile(dir, "2026/01.journal", validTx)); err != nil {
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
	if err := l.Do(t.Context(), "test write", badAppend); err == nil {
		t.Fatal("Do() with invalid append should have returned error")
	}

	// Existing file should be restored to its content after the first valid write.
	restored, _ := os.ReadFile(filepath.Join(dir, "2026/01.journal"))
	if string(restored) != string(originalContent) {
		t.Errorf("existing file not restored after failed write\ngot:\n%s\nwant:\n%s", restored, originalContent)
	}
	// Generation is 2: one bump for the successful write, one for the failed
	// write (failures invalidate caches that may have seen intermediate state).
	if got := l.Generation(); got != 2 {
		t.Errorf("Generation() = %d, want 2", got)
	}
}

func TestTxLock_OnCommit(t *testing.T) {
	validTx := "2026-01-15 AMAZON\n    expenses:food  $10.00\n    assets:checking  $-10.00\n\n"
	invalidTx := "2026-01-15 BROKEN\n    expenses:food  $10.00\n    assets:checking  $5.00\n\n"

	dir := setupDataDir(t)
	l := mustTxLock(t, dir)

	var mu sync.Mutex
	var gens []uint64
	l.OnCommit(func(gen uint64) {
		mu.Lock()
		defer mu.Unlock()
		gens = append(gens, gen)
	})

	// A failed write must not fire the hook, even though it still bumps the
	// generation to invalidate readers of the intermediate state.
	if err := l.Do(t.Context(), "bad write", addMonthFile(dir, "2026/01.journal", invalidTx)); err == nil {
		t.Fatal("expected check error")
	}

	if err := l.Do(t.Context(), "good write", addMonthFile(dir, "2026/01.journal", validTx)); err != nil {
		t.Fatalf("Do() failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(gens) != 1 || gens[0] != l.Generation() {
		t.Fatalf("OnCommit hook calls = %v, want exactly one call with generation %d", gens, l.Generation())
	}
}

func TestTxLock_DoWith(t *testing.T) {
	validTx := "2026-01-15 AMAZON\n    expenses:food  $10.00\n    assets:checking  $-10.00\n\n"
	invalidTx := "2026-01-15 BROKEN\n    expenses:food  $10.00\n    assets:checking  $5.00\n\n"

	t.Run("extra file reverted on check failure", func(t *testing.T) {
		// fn writes a valid-looking extra file AND an invalid journal.
		// hledger check fails → journal reverted, extra file also reverted.
		dir := setupDataDir(t)
		l := mustTxLock(t, dir)
		extraPath := filepath.Join(dir, "upload.csv")

		err := l.DoWith(t.Context(), "test", []string{extraPath}, func() error {
			_ = os.WriteFile(extraPath, []byte("csv data"), 0644)
			return addMonthFile(dir, "2026/01.journal", invalidTx)()
		})
		if err == nil {
			t.Fatal("DoWith() should have returned check error")
		}
		// Journal file should not exist.
		if _, statErr := os.Stat(filepath.Join(dir, "2026/01.journal")); !os.IsNotExist(statErr) {
			t.Error("journal file should have been reverted")
		}
		// Extra file should have been removed (was absent before fn).
		if _, statErr := os.Stat(extraPath); !os.IsNotExist(statErr) {
			t.Error("extra file should have been removed after check failure")
		}
	})

	t.Run("existing extra file restored on check failure", func(t *testing.T) {
		// extra file exists before fn; fn overwrites it AND writes invalid journal.
		// On failure, extra file should be restored to its original content.
		dir := setupDataDir(t)
		l := mustTxLock(t, dir)
		extraPath := filepath.Join(dir, "upload.csv")
		originalContent := []byte("original csv")
		if err := os.WriteFile(extraPath, originalContent, 0644); err != nil {
			t.Fatal(err)
		}

		err := l.DoWith(t.Context(), "test", []string{extraPath}, func() error {
			_ = os.WriteFile(extraPath, []byte("new csv"), 0644)
			return addMonthFile(dir, "2026/01.journal", invalidTx)()
		})
		if err == nil {
			t.Fatal("DoWith() should have returned check error")
		}
		restored, readErr := os.ReadFile(extraPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(restored) != string(originalContent) {
			t.Errorf("extra file not restored: got %q, want %q", restored, originalContent)
		}
	})

	t.Run("extra file reverted on fn error", func(t *testing.T) {
		// fn writes extra file then returns an error — extra file should be removed.
		dir := setupDataDir(t)
		l := mustTxLock(t, dir)
		extraPath := filepath.Join(dir, "rules.json")

		err := l.DoWith(t.Context(), "test", []string{extraPath}, func() error {
			_ = os.WriteFile(extraPath, []byte(`[]`), 0644)
			return errors.New("fn failed")
		})
		if err == nil {
			t.Fatal("DoWith() should have returned fn error")
		}
		if _, statErr := os.Stat(extraPath); !os.IsNotExist(statErr) {
			t.Error("extra file should have been removed after fn error")
		}
	})

	t.Run("successful DoWith keeps extra file", func(t *testing.T) {
		dir := setupDataDir(t)
		l := mustTxLock(t, dir)
		extraPath := filepath.Join(dir, "rules.json")

		err := l.DoWith(t.Context(), "test", []string{extraPath}, func() error {
			_ = os.WriteFile(extraPath, []byte(`[]`), 0644)
			return addMonthFile(dir, "2026/01.journal", validTx)()
		})
		if err != nil {
			t.Fatalf("DoWith() error = %v", err)
		}
		if _, statErr := os.Stat(extraPath); statErr != nil {
			t.Error("extra file should exist after successful write")
		}
		if got := l.Generation(); got != 1 {
			t.Errorf("Generation() = %d, want 1", got)
		}
	})

	t.Run("existing extra file deleted by fn is restored on check failure", func(t *testing.T) {
		// extra file exists before fn; fn deletes it AND writes an invalid journal.
		// On check failure the extra file should be restored with its original content.
		dir := setupDataDir(t)
		l := mustTxLock(t, dir)
		extraPath := filepath.Join(dir, "rules.json")
		originalContent := []byte(`[{"id":"aabbccdd","pattern":"AMAZON"}]`)
		if writeErr := os.WriteFile(extraPath, originalContent, 0644); writeErr != nil {
			t.Fatal(writeErr)
		}

		err := l.DoWith(t.Context(), "test", []string{extraPath}, func() error {
			if removeErr := os.Remove(extraPath); removeErr != nil {
				return removeErr
			}
			return addMonthFile(dir, "2026/01.journal", invalidTx)()
		})
		if err == nil {
			t.Fatal("DoWith() should have returned check error")
		}
		restored, readErr := os.ReadFile(extraPath)
		if readErr != nil {
			t.Fatalf("extra file should exist after revert: %v", readErr)
		}
		if string(restored) != string(originalContent) {
			t.Errorf("extra file not restored: got %q, want %q", restored, originalContent)
		}
	})

	t.Run("absent extra not created by fn is fine on revert", func(t *testing.T) {
		// extra path declared but fn never creates it; check fails; revert should not error.
		dir := setupDataDir(t)
		l := mustTxLock(t, dir)
		extraPath := filepath.Join(dir, "never-created.csv")

		err := l.DoWith(t.Context(), "test", []string{extraPath}, func() error {
			return addMonthFile(dir, "2026/01.journal", invalidTx)()
		})
		if err == nil {
			t.Fatal("DoWith() should have returned check error")
		}
		if _, statErr := os.Stat(extraPath); !os.IsNotExist(statErr) {
			t.Error("undeclared file should not exist")
		}
	})
}

func TestTxLock_DoWith_ErrNoChanges(t *testing.T) {
	// fn returns ErrNoChanges: DoWith must treat it as success but skip
	// hledger check, the generation bump, and any file mutation it implies.
	dir := setupDataDir(t)
	l := mustTxLock(t, dir)
	extraPath := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(extraPath, []byte(`[]`), 0644); err != nil {
		t.Fatal(err)
	}

	var hookCalls int
	l.OnCommit(func(gen uint64) { hookCalls++ })

	err := l.DoWith(t.Context(), "no-op", []string{extraPath}, func() error {
		return txlock.ErrNoChanges
	})
	if err != nil {
		t.Fatalf("DoWith() with ErrNoChanges should succeed, got %v", err)
	}
	if got := l.Generation(); got != 0 {
		t.Errorf("Generation() = %d, want 0 (no bump on ErrNoChanges)", got)
	}
	if hookCalls != 0 {
		t.Errorf("commit hook fired %d times, want 0 on ErrNoChanges", hookCalls)
	}
	content, readErr := os.ReadFile(extraPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != `[]` {
		t.Errorf("extra file content changed: got %q", content)
	}
}

func TestTxLock_DoConfig_RefusesJournalPath(t *testing.T) {
	dir := setupDataDir(t)
	l := txlock.New(dir, nil)

	journalPath := filepath.Join(dir, "2026/01.journal")
	called := false
	err := l.DoConfig(t.Context(), "bad call site", []string{journalPath}, func() error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("DoConfig() with a .journal path should return an error")
	}
	if called {
		t.Error("fn should not run when DoConfig refuses a .journal path")
	}
}

func TestTxLock_DoConfig_SkipsCheckAndGenerationBump(t *testing.T) {
	// Pass a nil hledger client: if DoConfig ever called client.Check, this
	// would panic, proving the skip.
	dir := setupDataDir(t)
	l := txlock.New(dir, nil)
	configPath := filepath.Join(dir, "config.toml")

	var hookCalls int
	l.OnCommit(func(gen uint64) { hookCalls++ })

	err := l.DoConfig(t.Context(), "set setting", []string{configPath}, func() error {
		return os.WriteFile(configPath, []byte("timezone = \"UTC\"\n"), 0644)
	})
	if err != nil {
		t.Fatalf("DoConfig() error = %v", err)
	}
	if got := l.Generation(); got != 0 {
		t.Errorf("Generation() = %d, want 0 (DoConfig must not bump generation)", got)
	}
	if hookCalls != 0 {
		t.Errorf("commit hook fired %d times, want 0 (DoConfig never bumps generation)", hookCalls)
	}
	content, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "timezone = \"UTC\"\n" {
		t.Errorf("config.toml content = %q, want written value", content)
	}
}

func TestTxLock_DoConfig_RevertsOnFnError(t *testing.T) {
	dir := setupDataDir(t)
	l := txlock.New(dir, nil)

	t.Run("new file removed on fn error", func(t *testing.T) {
		configPath := filepath.Join(dir, "config.toml")
		err := l.DoConfig(t.Context(), "test", []string{configPath}, func() error {
			if writeErr := os.WriteFile(configPath, []byte("bad"), 0644); writeErr != nil {
				return writeErr
			}
			return errors.New("fn failed")
		})
		if err == nil {
			t.Fatal("DoConfig() should have returned fn error")
		}
		if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
			t.Error("new config file should have been removed after fn error")
		}
	})

	t.Run("existing file restored on fn error", func(t *testing.T) {
		rulesPath := filepath.Join(dir, "rules.json")
		original := []byte(`[{"id":"aabbccdd","pattern":"AMAZON"}]`)
		if err := os.WriteFile(rulesPath, original, 0644); err != nil {
			t.Fatal(err)
		}
		err := l.DoConfig(t.Context(), "test", []string{rulesPath}, func() error {
			if writeErr := os.WriteFile(rulesPath, []byte(`[]`), 0644); writeErr != nil {
				return writeErr
			}
			return errors.New("fn failed")
		})
		if err == nil {
			t.Fatal("DoConfig() should have returned fn error")
		}
		restored, readErr := os.ReadFile(rulesPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(restored) != string(original) {
			t.Errorf("rules.json not restored: got %q, want %q", restored, original)
		}
	})
}

func TestTxLock_DoConfig_ErrNoChanges(t *testing.T) {
	dir := setupDataDir(t)
	l := txlock.New(dir, nil)
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	err := l.DoConfig(t.Context(), "no-op", []string{configPath}, func() error {
		return txlock.ErrNoChanges
	})
	if err != nil {
		t.Fatalf("DoConfig() with ErrNoChanges should succeed, got %v", err)
	}
	content, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "original" {
		t.Errorf("config.toml content = %q, want unchanged", content)
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
			errs[i] = l.Do(t.Context(), "test write", addMonthFile(dir, months[i], txns[i]))
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
