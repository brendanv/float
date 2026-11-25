package ledger_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	floatv1 "github.com/brendanv/float/gen/float/v1"
	"github.com/brendanv/float/internal/gitsnap"
	serverledger "github.com/brendanv/float/internal/server/ledger"
	"github.com/brendanv/float/internal/testgen"
)

// journalText concatenates every *.journal file under dir.
func journalText(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".journal") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			b.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func snapMessages(t *testing.T, snap *gitsnap.Repo) []string {
	t.Helper()
	snaps, err := snap.List(t.Context(), 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	out := make([]string, len(snaps))
	for i, s := range snaps {
		out[i] = strings.TrimSpace(s.Message)
	}
	return out
}

// restoreFixture returns a handler whose data dir has a "seed" snapshot
// followed by a snapshot that added a "Coffee Restore Test" transaction.
func restoreFixture(t *testing.T, seed int64) (h *serverledger.Handler, snap *gitsnap.Repo, dir, seedHash string) {
	t.Helper()
	dir = testgen.GenerateDataDir(t, testgen.Options{Seed: seed, NumTxns: 3, WithFIDs: true})
	h, snap = mustRealHandlerWithSnap(t, dir)
	if err := snap.Commit(t.Context(), "seed"); err != nil {
		t.Fatalf("seed commit: %v", err)
	}
	seedHash, err := snap.Head()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.AddTransaction(t.Context(), connect.NewRequest(&floatv1.AddTransactionRequest{
		Description: "Coffee Restore Test",
		Date:        "2026-01-15",
		Postings: []*floatv1.PostingInput{
			{Account: "expenses:food", Commodity: "USD", Quantity: "4.50"},
			{Account: "assets:checking"},
		},
	})); err != nil {
		t.Fatalf("AddTransaction: %v", err)
	}
	if !strings.Contains(journalText(t, dir), "Coffee Restore Test") {
		t.Fatal("fixture transaction not written")
	}
	return h, snap, dir, seedHash
}

func TestRestoreSnapshot_ForwardKeepsHistory(t *testing.T) {
	h, snap, dir, seedHash := restoreFixture(t, 700)
	before := snapMessages(t, snap)

	if _, err := h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: seedHash})); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}

	if strings.Contains(journalText(t, dir), "Coffee Restore Test") {
		t.Error("restore did not remove the later transaction")
	}
	after := snapMessages(t, snap)
	if len(after) != len(before)+1 {
		t.Fatalf("history = %q, want one new snapshot on top of %q", after, before)
	}
	if want := `restore snapshot ` + seedHash[:8] + ` ("seed")`; after[0] != want {
		t.Errorf("restore commit message = %q, want %q", after[0], want)
	}
	if strings.Join(after[1:], "|") != strings.Join(before, "|") {
		t.Errorf("earlier history changed: got %q, want %q", after[1:], before)
	}

	// The pre-restore snapshot (the AddTransaction write) can be restored back.
	if _, err := h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: mustSnapHash(t, snap, 1)})); err != nil {
		t.Fatalf("RestoreSnapshot back: %v", err)
	}
	if !strings.Contains(journalText(t, dir), "Coffee Restore Test") {
		t.Error("restoring the pre-restore snapshot did not bring the transaction back")
	}
}

func mustSnapHash(t *testing.T, snap *gitsnap.Repo, i int) string {
	t.Helper()
	snaps, err := snap.List(t.Context(), 0)
	if err != nil || len(snaps) <= i {
		t.Fatalf("List: %d snapshots, err %v", len(snaps), err)
	}
	return snaps[i].Hash
}

func TestRestoreSnapshot_HardResetDiscardsHistory(t *testing.T) {
	h, snap, dir, seedHash := restoreFixture(t, 701)

	if _, err := h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: seedHash, HardReset: true})); err != nil {
		t.Fatalf("RestoreSnapshot hard: %v", err)
	}

	if strings.Contains(journalText(t, dir), "Coffee Restore Test") {
		t.Error("hard reset did not remove the later transaction")
	}
	if head, _ := snap.Head(); head != seedHash {
		t.Errorf("HEAD = %s, want %s", head, seedHash)
	}
	if got := snapMessages(t, snap); got[0] != "seed" {
		t.Errorf("history = %q, want it to end at seed", got)
	}
}

func TestRestoreSnapshot_RestoringHeadIsNoop(t *testing.T) {
	h, snap, _, _ := restoreFixture(t, 702)
	before := snapMessages(t, snap)
	head, _ := snap.Head()
	if _, err := h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: head})); err != nil {
		t.Fatalf("RestoreSnapshot HEAD: %v", err)
	}
	if after := snapMessages(t, snap); len(after) != len(before) {
		t.Errorf("restoring HEAD added a snapshot: %q", after)
	}
}

// TestRestoreSnapshot_InvalidTargetIsReverted restores a snapshot whose
// journal fails hledger check: the files must come back exactly and history
// must not change, in both modes.
func TestRestoreSnapshot_InvalidTargetIsReverted(t *testing.T) {
	for _, hard := range []bool{false, true} {
		t.Run(map[bool]string{false: "forward", true: "hard_reset"}[hard], func(t *testing.T) {
			h, snap, dir, _ := restoreFixture(t, 703)
			goodMain, err := os.ReadFile(filepath.Join(dir, "main.journal"))
			if err != nil {
				t.Fatal(err)
			}

			// Snapshot a broken journal (plus a new file) directly, bypassing
			// txlock, then repair it so HEAD is valid again.
			brokenPath := filepath.Join(dir, "broken.journal")
			if err := os.WriteFile(brokenPath, []byte("2026-01-01 broken\n    a  1 USD\n    b  2 USD\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "main.journal"), append(append([]byte{}, goodMain...), []byte("\ninclude broken.journal\n")...), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := snap.Commit(t.Context(), "broken"); err != nil {
				t.Fatal(err)
			}
			brokenHash, _ := snap.Head()
			if err := os.Remove(brokenPath); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "main.journal"), goodMain, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := snap.Commit(t.Context(), "repaired"); err != nil {
				t.Fatal(err)
			}
			head, _ := snap.Head()
			beforeText := journalText(t, dir)
			beforeHistory := snapMessages(t, snap)

			_, err = h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: brokenHash, HardReset: hard}))
			if err == nil {
				t.Fatal("expected restore of an invalid snapshot to fail")
			}
			if got := journalText(t, dir); got != beforeText {
				t.Errorf("journal files not reverted after failed restore")
			}
			if _, err := os.Stat(brokenPath); !os.IsNotExist(err) {
				t.Errorf("broken.journal left behind after failed restore (stat err %v)", err)
			}
			if got, _ := snap.Head(); got != head {
				t.Errorf("HEAD moved to %s after failed restore, want %s", got, head)
			}
			if got := snapMessages(t, snap); strings.Join(got, "|") != strings.Join(beforeHistory, "|") {
				t.Errorf("history changed after failed restore: %q", got)
			}
		})
	}
}

func TestRestoreSnapshot_BadHash(t *testing.T) {
	h, _, _, _ := restoreFixture(t, 704)
	for _, hash := range []string{"", "deadbeef", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"} {
		_, err := h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: hash}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("hash %q: code = %v (err %v), want InvalidArgument", hash, connect.CodeOf(err), err)
		}
	}
}

// TestRestoreSnapshot_RecordsUncommittedChangesFirst covers a data dir left
// dirty by a failed snapshot commit: the stray changes are snapshotted on
// their own before the restore, so they are not folded into it or lost.
func TestRestoreSnapshot_RecordsUncommittedChangesFirst(t *testing.T) {
	h, snap, dir, seedHash := restoreFixture(t, 705)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("stray"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := h.RestoreSnapshot(t.Context(), connect.NewRequest(&floatv1.RestoreSnapshotRequest{Hash: seedHash})); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	got := snapMessages(t, snap)
	if !strings.HasPrefix(got[0], "restore snapshot ") || !strings.HasPrefix(got[1], "float: recovery snapshot") {
		t.Errorf("history = %q, want restore on top of a recovery snapshot", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); !os.IsNotExist(err) {
		t.Errorf("notes.txt should be gone after restoring seed (stat err %v)", err)
	}
}
