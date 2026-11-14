package gitsnap

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func writeTestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitAndHash(t *testing.T, r *Repo, msg string) string {
	t.Helper()
	if err := r.Commit(t.Context(), msg); err != nil {
		t.Fatalf("Commit %q: %v", msg, err)
	}
	head, err := r.repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return head.Hash().String()
}

func headHash(t *testing.T, r *Repo) string {
	t.Helper()
	head, err := r.repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return head.Hash().String()
}

func treeHash(t *testing.T, r *Repo, hash string) string {
	t.Helper()
	c, err := r.repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		t.Fatalf("CommitObject %s: %v", hash, err)
	}
	return c.TreeHash.String()
}

// assertFiles checks each rel path has the wanted content; an empty want
// means the file must not exist.
func assertFiles(t *testing.T, dir string, want map[string]string) {
	t.Helper()
	for rel, content := range want {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		switch {
		case content == "" && err == nil:
			t.Errorf("%s exists with %q, want absent", rel, got)
		case content == "" && os.IsNotExist(err):
		case err != nil:
			t.Errorf("read %s: %v", rel, err)
		case string(got) != content:
			t.Errorf("%s = %q, want %q", rel, got, content)
		}
	}
}

// twoSnapshotRepo builds history init → A → B where B modifies, deletes, and
// adds files (including in a nested dir), and config.toml exists locally.
func twoSnapshotRepo(t *testing.T) (r *Repo, dir, hashA, hashB string) {
	t.Helper()
	dir = t.TempDir()
	r, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	writeTestFile(t, dir, "main.journal", "journal-A")
	writeTestFile(t, dir, "rules/bank.rules", "rules-A")
	writeTestFile(t, dir, "old.txt", "only-in-A")
	hashA = commitAndHash(t, r, "commit A")

	writeTestFile(t, dir, "main.journal", "journal-B")
	writeTestFile(t, dir, "rules/bank.rules", "rules-B")
	if err := os.Remove(filepath.Join(dir, "old.txt")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "uploads/new.csv", "only-in-B")
	writeTestFile(t, dir, "config.toml", "local-config")
	hashB = commitAndHash(t, r, "commit B")
	return r, dir, hashA, hashB
}

func TestRestore_NonDestructiveAndUndoable(t *testing.T) {
	ctx := t.Context()
	r, dir, hashA, hashB := twoSnapshotRepo(t)

	// Restore to A: files match A, HEAD stays on B.
	if err := r.Restore(ctx, hashA); err != nil {
		t.Fatalf("Restore A: %v", err)
	}
	assertFiles(t, dir, map[string]string{
		"main.journal":     "journal-A",
		"rules/bank.rules": "rules-A",
		"old.txt":          "only-in-A",
		"uploads/new.csv":  "",
		"config.toml":      "local-config",
	})
	if _, err := os.Stat(filepath.Join(dir, "uploads")); !os.IsNotExist(err) {
		t.Errorf("empty uploads/ dir should be removed, stat err = %v", err)
	}
	if got := headHash(t, r); got != hashB {
		t.Errorf("HEAD moved by restore: got %s, want %s", got, hashB)
	}

	restoreA := commitAndHash(t, r, "restore snapshot "+hashA)
	snaps, err := r.List(ctx, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var hashes []string
	for _, s := range snaps {
		hashes = append(hashes, s.Hash)
	}
	if len(hashes) != 4 || hashes[0] != restoreA || hashes[1] != hashB || hashes[2] != hashA {
		t.Fatalf("history after restore = %v, want [restoreA %s, B %s, A %s, init]", hashes, restoreA, hashB, hashA)
	}
	if snaps[0].Message != "restore snapshot "+hashA {
		t.Errorf("top message = %q", snaps[0].Message)
	}

	// Undo: restore back to B, which is still reachable.
	if err := r.Restore(ctx, hashB); err != nil {
		t.Fatalf("Restore B: %v", err)
	}
	assertFiles(t, dir, map[string]string{
		"main.journal":     "journal-B",
		"rules/bank.rules": "rules-B",
		"old.txt":          "",
		"uploads/new.csv":  "only-in-B",
		"config.toml":      "local-config",
	})
	restoreB := commitAndHash(t, r, "restore snapshot "+hashB)
	snaps, err = r.List(ctx, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 5 || snaps[0].Hash != restoreB || snaps[1].Hash != restoreA {
		t.Errorf("history after undo has %d entries, top %v", len(snaps), snaps[:min(2, len(snaps))])
	}

	// The undo commit's tree equals B's tree.
	if got, want := treeHash(t, r, restoreB), treeHash(t, r, hashB); got != want {
		t.Errorf("undo commit tree = %s, want B's tree %s", got, want)
	}
	if got, want := treeHash(t, r, restoreA), treeHash(t, r, hashA); got != want {
		t.Errorf("restore commit tree = %s, want A's tree %s", got, want)
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if st, err := wt.Status(); err != nil || !st.IsClean() {
		t.Errorf("worktree not clean after restore commit: %v %v", st, err)
	}
}

func TestPlanRestore_Paths(t *testing.T) {
	r, dir, hashA, hashB := twoSnapshotRepo(t)
	all := []string{".gitignore", "main.journal", "old.txt", "rules/bank.rules", "uploads/new.csv"}
	tests := []struct {
		name string
		hash string
		want []string
	}{
		{name: "older_snapshot_unions_head_and_target", hash: hashA, want: all},
		{name: "head_itself_covers_tracked_files", hash: hashB, want: []string{".gitignore", "main.journal", "rules/bank.rules", "uploads/new.csv"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := r.PlanRestore(t.Context(), tt.hash)
			if err != nil {
				t.Fatalf("PlanRestore: %v", err)
			}
			var want []string
			for _, rel := range tt.want {
				want = append(want, filepath.Join(dir, filepath.FromSlash(rel)))
			}
			if got := plan.Paths(); !slices.Equal(got, want) {
				t.Errorf("Paths() =\n%v\nwant\n%v", got, want)
			}
		})
	}
}

func TestApplyRestore_StalePlan(t *testing.T) {
	r, dir, hashA, _ := twoSnapshotRepo(t)
	plan, err := r.PlanRestore(t.Context(), hashA)
	if err != nil {
		t.Fatalf("PlanRestore: %v", err)
	}
	writeTestFile(t, dir, "rules/other.rules", "new rules")
	commitAndHash(t, r, "concurrent write")

	if err := r.ApplyRestore(t.Context(), plan); !errors.Is(err, ErrStaleRestorePlan) {
		t.Fatalf("ApplyRestore err = %v, want ErrStaleRestorePlan", err)
	}
	assertFiles(t, dir, map[string]string{"main.journal": "journal-B", "rules/other.rules": "new rules"})
}

// TestRestore_ReHealsOldGitignore guards against an old snapshot's .gitignore
// (missing newer entries) letting the post-restore commit capture secrets.
func TestRestore_ReHealsOldGitignore(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	r, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	writeTestFile(t, dir, ".gitignore", "config.toml\nfloat.key\n")
	writeTestFile(t, dir, "main.journal", "A")
	hashA := commitAndHash(t, r, "old gitignore")

	if err := ensureGitignore(dir); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "main.journal", "B")
	commitAndHash(t, r, "healed gitignore")
	writeTestFile(t, dir, "ssh_host_key", "PRIVATE")

	if err := r.Restore(ctx, hashA); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gi), "ssh_host_key") {
		t.Errorf(".gitignore lost ssh_host_key after restore:\n%s", gi)
	}
	assertFiles(t, dir, map[string]string{"main.journal": "A", "ssh_host_key": "PRIVATE"})
	commitAndHash(t, r, "restore")
	head, _ := r.repo.Head()
	commit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.File("ssh_host_key"); err == nil {
		t.Error("ssh_host_key was committed after restore")
	}
}
