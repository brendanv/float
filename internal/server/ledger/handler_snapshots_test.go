package ledger_test

import (
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/gitsnap"
	"github.com/brendanv/float/internal/testgen"
)

func writeDataFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readDataFile(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func snapHead(t *testing.T, snap *gitsnap.Repo) (string, int) {
	t.Helper()
	snaps, err := snap.List(t.Context(), 1000)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return snaps[0].Hash, len(snaps)
}

func commitSnap(t *testing.T, snap *gitsnap.Repo, msg string) string {
	t.Helper()
	if err := snap.Commit(t.Context(), msg); err != nil {
		t.Fatalf("Commit %q: %v", msg, err)
	}
	hash, _ := snapHead(t, snap)
	return hash
}

func TestRestoreSnapshot_RecordsNewSnapshotAndIsUndoable(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 900, NumTxns: 3})
	h, snap := mustRealHandlerWithSnap(t, dir)

	writeDataFile(t, dir, "rules/bank.rules", "rules-old\n")
	oldHash := commitSnap(t, snap, "old state")
	oldJournal := readDataFile(t, dir, "main.journal")

	writeDataFile(t, dir, "main.journal", oldJournal+"\n; newer comment\n")
	writeDataFile(t, dir, "rules/bank.rules", "rules-new\n")
	writeDataFile(t, dir, "uploads/new.csv", "date,amount\n")
	newHash := commitSnap(t, snap, "new state")

	if _, err := h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: oldHash})); err != nil {
		t.Fatalf("RestoreSnapshot(old): %v", err)
	}
	if got := readDataFile(t, dir, "main.journal"); got != oldJournal {
		t.Errorf("main.journal not restored:\n%s", got)
	}
	if got := readDataFile(t, dir, "rules/bank.rules"); got != "rules-old\n" {
		t.Errorf("rules/bank.rules = %q, want rules-old", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "uploads/new.csv")); !os.IsNotExist(err) {
		t.Errorf("uploads/new.csv should be removed, stat err = %v", err)
	}

	resp, err := h.ListSnapshots(t.Context(), connect.NewRequest(&floatv1.ListSnapshotsRequest{}))
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	snaps := resp.Msg.Snapshots
	if len(snaps) < 3 || snaps[0].Message != "restore snapshot "+oldHash || snaps[1].Hash != newHash || snaps[2].Hash != oldHash {
		t.Fatalf("snapshots after restore = %v, want restore commit on top of new and old", snaps)
	}

	// Undo the restore by restoring the newer snapshot.
	if _, err := h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: newHash})); err != nil {
		t.Fatalf("RestoreSnapshot(new): %v", err)
	}
	if got := readDataFile(t, dir, "rules/bank.rules"); got != "rules-new\n" {
		t.Errorf("rules/bank.rules after undo = %q, want rules-new", got)
	}
	if got := readDataFile(t, dir, "uploads/new.csv"); got != "date,amount\n" {
		t.Errorf("uploads/new.csv after undo = %q", got)
	}
}

func TestRestoreSnapshot_CheckFailureFullyReverts(t *testing.T) {
	dir := testgen.GenerateDataDir(t, testgen.Options{Seed: 901, NumTxns: 3})
	h, snap := mustRealHandlerWithSnap(t, dir)
	goodJournal := readDataFile(t, dir, "main.journal")

	// A snapshot whose journal fails hledger check.
	writeDataFile(t, dir, "main.journal", goodJournal+"\nthis is not { valid journal syntax\n")
	writeDataFile(t, dir, "rules/bank.rules", "rules-broken\n")
	writeDataFile(t, dir, "rules/only-broken.rules", "only in broken\n")
	brokenHash := commitSnap(t, snap, "broken state")

	// Current, valid state.
	writeDataFile(t, dir, "main.journal", goodJournal)
	writeDataFile(t, dir, "rules/bank.rules", "rules-current\n")
	if err := os.Remove(filepath.Join(dir, "rules/only-broken.rules")); err != nil {
		t.Fatal(err)
	}
	writeDataFile(t, dir, "uploads/current.csv", "current csv\n")
	headBefore := commitSnap(t, snap, "current state")
	_, countBefore := snapHead(t, snap)

	_, err := h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: brokenHash}))
	if err == nil {
		t.Fatal("RestoreSnapshot of a snapshot failing hledger check should error")
	}

	if got := readDataFile(t, dir, "main.journal"); got != goodJournal {
		t.Errorf("main.journal not reverted:\n%s", got)
	}
	if got := readDataFile(t, dir, "rules/bank.rules"); got != "rules-current\n" {
		t.Errorf("rules/bank.rules = %q, want reverted to rules-current", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules/only-broken.rules")); !os.IsNotExist(err) {
		t.Errorf("rules/only-broken.rules should be removed by revert, stat err = %v", err)
	}
	if got := readDataFile(t, dir, "uploads/current.csv"); got != "current csv\n" {
		t.Errorf("uploads/current.csv = %q, want restored", got)
	}
	head, count := snapHead(t, snap)
	if head != headBefore || count != countBefore {
		t.Errorf("snapshot history changed: head %s (want %s), count %d (want %d)", head, headBefore, count, countBefore)
	}
	// Disk matches HEAD again: nothing left to commit.
	if err := snap.Commit(t.Context(), "probe"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if head, _ := snapHead(t, snap); head != headBefore {
		t.Error("worktree was left dirty after the reverted restore")
	}
}
