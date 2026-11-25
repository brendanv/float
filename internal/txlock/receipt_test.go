package txlock_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/txlock"
)

func mustSnappedTxLock(t *testing.T) (*txlock.TxLock, *gitsnap.Repo, string) {
	t.Helper()
	dir := setupDataDir(t)
	lock := mustTxLock(t, dir)
	snap, err := gitsnap.New(dir)
	if err != nil {
		t.Fatalf("gitsnap.New: %v", err)
	}
	if err := snap.Commit(t.Context(), "seed"); err != nil {
		t.Fatal(err)
	}
	lock.SetSnap(snap)
	return lock, snap, dir
}

func writeFileFn(path, content string) func() error {
	return func() error { return os.WriteFile(path, []byte(content), 0o644) }
}

func TestReceipt(t *testing.T) {
	addTxn := "\n2026-01-01 coffee\n    expenses:food  1 USD\n    assets:checking\n"

	t.Run("one_journal_write_is_undoable", func(t *testing.T) {
		lock, snap, dir := mustSnappedTxLock(t)
		ctx, r := txlock.WithReceipt(t.Context())
		if err := lock.Do(ctx, "write", addMonthFile(dir, "2026/01.journal", addTxn)); err != nil {
			t.Fatal(err)
		}
		got, ok := r.Undoable()
		head, _ := snap.Head()
		if !ok || got != head {
			t.Errorf("Undoable() = %q, %v; want HEAD %q, true", got, ok, head)
		}
	})

	t.Run("tracked_config_write_is_undoable", func(t *testing.T) {
		lock, _, dir := mustSnappedTxLock(t)
		rulesPath := filepath.Join(dir, "rules.json")
		ctx, r := txlock.WithReceipt(t.Context())
		if err := lock.DoConfig(ctx, "rules", []string{rulesPath}, writeFileFn(rulesPath, "[]")); err != nil {
			t.Fatal(err)
		}
		if _, ok := r.Undoable(); !ok {
			t.Error("rules.json write should be undoable")
		}
	})

	t.Run("config_toml_write_is_not_undoable", func(t *testing.T) {
		lock, _, dir := mustSnappedTxLock(t)
		cfg := filepath.Join(dir, "config.toml")
		ctx, r := txlock.WithReceipt(t.Context())
		err := lock.DoWith(ctx, "journal+config", []string{cfg}, func() error {
			if err := writeFileFn(cfg, "x = 1\n")(); err != nil {
				return err
			}
			return addMonthFile(dir, "2026/01.journal", addTxn)()
		})
		if err != nil {
			t.Fatal(err)
		}
		if s, ok := r.Undoable(); ok {
			t.Errorf("write declaring config.toml should not be undoable, got %q", s)
		}
	})

	t.Run("two_writes_are_not_undoable", func(t *testing.T) {
		lock, _, dir := mustSnappedTxLock(t)
		ctx, r := txlock.WithReceipt(t.Context())
		if err := lock.Do(ctx, "one", addMonthFile(dir, "2026/01.journal", addTxn)); err != nil {
			t.Fatal(err)
		}
		if err := lock.Do(ctx, "two", addMonthFile(dir, "2026/02.journal", addTxn)); err != nil {
			t.Fatal(err)
		}
		if _, ok := r.Undoable(); ok {
			t.Error("two commits under one receipt should not be undoable")
		}
	})

	t.Run("failed_and_noop_writes_record_nothing", func(t *testing.T) {
		lock, _, dir := mustSnappedTxLock(t)
		ctx, r := txlock.WithReceipt(t.Context())
		_ = lock.Do(ctx, "fails", func() error { return errors.New("boom") })
		_ = lock.Do(ctx, "noop", func() error { return txlock.ErrNoChanges })
		bad := "\n2026-01-01 unbalanced\n    expenses:food  1 USD\n    assets:checking  1 USD\n"
		if err := lock.Do(ctx, "check fails", addMonthFile(dir, "2026/01.journal", bad)); err == nil {
			t.Fatal("expected hledger check failure")
		}
		if _, ok := r.Undoable(); ok {
			t.Error("no successful write, so nothing should be undoable")
		}
		// A later successful write is the receipt's only write.
		if err := lock.Do(ctx, "ok", addMonthFile(dir, "2026/03.journal", addTxn)); err != nil {
			t.Fatal(err)
		}
		if _, ok := r.Undoable(); !ok {
			t.Error("the one successful write should be undoable")
		}
	})

	t.Run("no_snapshots_is_not_undoable", func(t *testing.T) {
		dir := setupDataDir(t)
		lock := mustTxLock(t, dir)
		ctx, r := txlock.WithReceipt(t.Context())
		if err := lock.Do(ctx, "write", addMonthFile(dir, "2026/01.journal", addTxn)); err != nil {
			t.Fatal(err)
		}
		if _, ok := r.Undoable(); ok {
			t.Error("without a snapshot repo nothing is undoable")
		}
	})
}
