package gitsnap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew_InitializesRepo(t *testing.T) {
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf(".git not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
		t.Errorf(".gitignore not created: %v", err)
	}
}

func TestNew_OpensExisting(t *testing.T) {
	dir := t.TempDir()
	if _, err := New(dir); err != nil {
		t.Fatalf("first New: %v", err)
	}
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

func TestCommit_StagesAndCommits(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "test.journal"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "test commit"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	snaps, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) < 2 {
		t.Fatalf("expected >= 2 snapshots, got %d", len(snaps))
	}
	if snaps[0].Message != "test commit" {
		t.Errorf("expected message %q, got %q", "test commit", snaps[0].Message)
	}
}

func TestCommit_MessageIsCapped(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "test.journal"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	longMsg := strings.Repeat("a", maxCommitMessageLen+25)
	if err := repo.Commit(ctx, longMsg); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	snaps, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := len(snaps[0].Message); got != maxCommitMessageLen {
		t.Fatalf("snapshot message len = %d, want %d", got, maxCommitMessageLen)
	}
	if !strings.HasSuffix(snaps[0].Message, "...") {
		t.Fatalf("expected truncated message to end with ellipsis, got %q", snaps[0].Message)
	}
}

func TestCommit_NothingToCommit(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := repo.Commit(ctx, "empty commit"); err != nil {
		t.Fatalf("Commit on clean tree: %v", err)
	}

	snaps, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("expected 1 snapshot (init only), got %d", len(snaps))
	}
}

func TestList_ReturnsInOrder(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	messages := []string{"first", "second", "third"}
	for i, msg := range messages {
		f := filepath.Join(dir, filepath.FromSlash("file"+string(rune('a'+i))+".journal"))
		if err := os.WriteFile(f, []byte(msg), 0644); err != nil {
			t.Fatal(err)
		}
		if err := repo.Commit(ctx, msg); err != nil {
			t.Fatalf("Commit %q: %v", msg, err)
		}
	}

	snaps, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) < 3 {
		t.Fatalf("expected >= 3 snapshots, got %d", len(snaps))
	}
	if snaps[0].Message != "third" {
		t.Errorf("expected newest first; got %q", snaps[0].Message)
	}
	if snaps[1].Message != "second" {
		t.Errorf("expected second; got %q", snaps[1].Message)
	}
}

func TestList_RespectsLimit(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for i := 0; i < 5; i++ {
		f := filepath.Join(dir, filepath.FromSlash("file.journal"))
		if err := os.WriteFile(f, []byte(string(rune('0'+i))), 0644); err != nil {
			t.Fatal(err)
		}
		if err := repo.Commit(ctx, "commit"); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}

	snaps, err := repo.List(ctx, 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 2 {
		t.Errorf("expected 2 snapshots with limit=2, got %d", len(snaps))
	}
}

// writeFiles writes path → content under dir (creating directories); an
// empty content deletes the file.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if content == "" {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// commitFiles writes files, commits them with msg and returns the new HEAD.
func commitFiles(t *testing.T, repo *Repo, dir, msg string, files map[string]string) string {
	t.Helper()
	writeFiles(t, dir, files)
	if err := repo.Commit(t.Context(), msg); err != nil {
		t.Fatalf("Commit %q: %v", msg, err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return head
}

func readFile(t *testing.T, dir, name string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

func snapshotMessages(t *testing.T, repo *Repo) []string {
	t.Helper()
	snaps, err := repo.List(t.Context(), 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	msgs := make([]string, len(snaps))
	for i, s := range snaps {
		msgs[i] = strings.TrimSpace(s.Message)
	}
	return msgs
}

// TestRestore_ForwardKeepsHistory restores an older snapshot covering a
// modified, an added, a deleted, and a nested file, and checks the result is
// committed on top of the existing history.
func TestRestore_ForwardKeepsHistory(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hashA := commitFiles(t, repo, dir, "commit A", map[string]string{
		"main.journal":    "version-A",
		"2026/01.journal": "january",
	})
	commitFiles(t, repo, dir, "commit B", map[string]string{
		"main.journal":    "version-B",
		"2026/01.journal": "",
		"uploads/x.csv":   "csv",
	})

	if err := repo.Restore(ctx, hashA, "restore A"); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if got, _ := readFile(t, dir, "main.journal"); got != "version-A" {
		t.Errorf("main.journal = %q, want version-A", got)
	}
	if got, ok := readFile(t, dir, "2026/01.journal"); !ok || got != "january" {
		t.Errorf("2026/01.journal = %q (exists %v), want january", got, ok)
	}
	if _, ok := readFile(t, dir, "uploads/x.csv"); ok {
		t.Error("uploads/x.csv should be removed by the restore")
	}
	want := []string{"restore A", "commit B", "commit A", "float: init"}
	if got := snapshotMessages(t, repo); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("history = %q, want %q", got, want)
	}
	// The restore commit records the deleted upload too, not just the
	// journal files, so the worktree is clean afterwards.
	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if st, err := wt.Status(); err != nil || !st.IsClean() {
		t.Errorf("worktree not clean after restore: %v %v", st, err)
	}
}

func mustHead(t *testing.T, repo *Repo) string {
	t.Helper()
	h, err := repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	return h
}

// TestRestore_RestoreCanBeRestoredAway checks the snapshot before a restore
// can itself be restored, i.e. a restore is never a one-way door.
func TestRestore_RestoreCanBeRestoredAway(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hashA := commitFiles(t, repo, dir, "commit A", map[string]string{"main.journal": "version-A"})
	hashB := commitFiles(t, repo, dir, "commit B", map[string]string{"main.journal": "version-B"})
	if err := repo.Restore(ctx, hashA, "restore A"); err != nil {
		t.Fatalf("Restore A: %v", err)
	}
	if err := repo.Restore(ctx, hashB, "restore B"); err != nil {
		t.Fatalf("Restore B: %v", err)
	}
	if got, _ := readFile(t, dir, "main.journal"); got != "version-B" {
		t.Errorf("main.journal = %q, want version-B", got)
	}
}

func TestRestore_PreservesIgnoredFiles(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	gitignore, _ := readFile(t, dir, ".gitignore")
	hashA := commitFiles(t, repo, dir, "commit A", map[string]string{"main.journal": "version-A"})
	commitFiles(t, repo, dir, "commit B", map[string]string{
		"main.journal": "version-B",
		"config.toml":  "[server]\nport=8080\n",
		".gitignore":   gitignore + "extra-local\n",
	})

	if err := repo.Restore(ctx, hashA, "restore A"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got, _ := readFile(t, dir, "config.toml"); got != "[server]\nport=8080\n" {
		t.Errorf("config.toml = %q, want it untouched", got)
	}
	if got, _ := readFile(t, dir, ".gitignore"); !strings.Contains(got, "extra-local") {
		t.Errorf(".gitignore lost newer entries: %q", got)
	}
}

func TestPlanRestore_Paths(t *testing.T) {
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hashA := commitFiles(t, repo, dir, "commit A", map[string]string{"main.journal": "A", "keep.journal": "same"})
	commitFiles(t, repo, dir, "commit B", map[string]string{"main.journal": "B", "new.journal": "new"})

	plan, err := repo.PlanRestore(t.Context(), hashA)
	if err != nil {
		t.Fatalf("PlanRestore: %v", err)
	}
	got := map[string]bool{}
	for _, p := range plan.Paths {
		got[p] = true
	}
	for _, name := range []string{"main.journal", "new.journal"} {
		if !got[filepath.Join(dir, name)] {
			t.Errorf("plan paths %q missing %s", plan.Paths, name)
		}
	}
	if len(plan.Paths) != 2 {
		t.Errorf("plan paths = %q, want exactly main.journal and new.journal", plan.Paths)
	}
	if plan.Target != hashA || strings.TrimSpace(plan.Message) != "commit A" {
		t.Errorf("plan target/message = %s %q", plan.Target, plan.Message)
	}

	// Restoring HEAD itself changes nothing.
	self, err := repo.PlanRestore(t.Context(), mustHead(t, repo))
	if err != nil {
		t.Fatalf("PlanRestore HEAD: %v", err)
	}
	if len(self.Paths) != 0 {
		t.Errorf("restoring HEAD: paths = %q, want none", self.Paths)
	}
}

func TestApplyRestore_RefusesStalePlan(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hashA := commitFiles(t, repo, dir, "commit A", map[string]string{"main.journal": "A"})
	commitFiles(t, repo, dir, "commit B", map[string]string{"main.journal": "B"})
	plan, err := repo.PlanRestore(ctx, hashA)
	if err != nil {
		t.Fatalf("PlanRestore: %v", err)
	}
	commitFiles(t, repo, dir, "commit C", map[string]string{"main.journal": "C"})

	if err := repo.ApplyRestore(ctx, plan); !errors.Is(err, ErrHeadMoved) {
		t.Fatalf("ApplyRestore after a new commit: err = %v, want ErrHeadMoved", err)
	}
	if got, _ := readFile(t, dir, "main.journal"); got != "C" {
		t.Errorf("main.journal = %q, want C (untouched)", got)
	}
}

func TestApplyRestore_RefusesDirtyWorktree(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hashA := commitFiles(t, repo, dir, "commit A", map[string]string{"main.journal": "A"})
	commitFiles(t, repo, dir, "commit B", map[string]string{"main.journal": "B"})
	writeFiles(t, dir, map[string]string{"other.journal": "uncommitted"})
	plan, err := repo.PlanRestore(ctx, hashA)
	if err != nil {
		t.Fatalf("PlanRestore: %v", err)
	}
	if err := repo.ApplyRestore(ctx, plan); !errors.Is(err, ErrDirty) {
		t.Fatalf("ApplyRestore on a dirty tree: err = %v, want ErrDirty", err)
	}
}

func TestRestore_InvalidHash(t *testing.T) {
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, hash := range []string{"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "deadbeef", "not-a-hash"} {
		if err := repo.Restore(t.Context(), hash, "x"); err == nil {
			t.Errorf("Restore(%q): expected error, got nil", hash)
		}
		if err := repo.ResetHard(t.Context(), hash); err == nil {
			t.Errorf("ResetHard(%q): expected error, got nil", hash)
		}
	}
}

func TestResetHard_DiscardsLaterHistory(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hashA := commitFiles(t, repo, dir, "commit A", map[string]string{"main.journal": "version-A"})
	commitFiles(t, repo, dir, "commit B", map[string]string{"main.journal": "version-B", "config.toml": "secret"})

	if err := repo.ResetHard(ctx, hashA); err != nil {
		t.Fatalf("ResetHard: %v", err)
	}
	if got, _ := readFile(t, dir, "main.journal"); got != "version-A" {
		t.Errorf("main.journal = %q, want version-A", got)
	}
	if got, _ := readFile(t, dir, "config.toml"); got != "secret" {
		t.Errorf("config.toml = %q, want it preserved", got)
	}
	want := []string{"commit A", "float: init"}
	if got := snapshotMessages(t, repo); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("history = %q, want %q", got, want)
	}
}

func TestRecoverUncommitted_DirtyTree(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "dirty.journal"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := repo.RecoverUncommitted(ctx); err != nil {
		t.Fatalf("RecoverUncommitted: %v", err)
	}

	snaps, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if snaps[0].Message != "float: recovery snapshot (uncommitted changes at startup)" {
		t.Errorf("unexpected recovery message: %q", snaps[0].Message)
	}
}

func TestDiff_ModifiedFile(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	journalPath := filepath.Join(dir, "main.journal")
	if err := os.WriteFile(journalPath, []byte("version-A\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "commit A"); err != nil {
		t.Fatalf("Commit A: %v", err)
	}
	if err := os.WriteFile(journalPath, []byte("version-B\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "commit B"); err != nil {
		t.Fatalf("Commit B: %v", err)
	}

	snaps, _ := repo.List(ctx, 1)
	files, err := repo.Diff(ctx, snaps[0].Hash)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "main.journal" {
		t.Errorf("path: got %q, want %q", files[0].Path, "main.journal")
	}
	if files[0].Change != ChangeModified {
		t.Errorf("change: got %v, want ChangeModified", files[0].Change)
	}
	if files[0].IsBinary {
		t.Errorf("expected text file, got binary")
	}
	if !strings.Contains(files[0].Patch, "-version-A") || !strings.Contains(files[0].Patch, "+version-B") {
		t.Errorf("patch missing markers:\n%s", files[0].Patch)
	}
}

func TestDiff_AddedFile(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "new.journal"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add new"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	snaps, _ := repo.List(ctx, 1)
	files, err := repo.Diff(ctx, snaps[0].Hash)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(files) != 1 || files[0].Path != "new.journal" || files[0].Change != ChangeAdded {
		t.Fatalf("expected single ChangeAdded for new.journal, got %+v", files)
	}
	if !strings.Contains(files[0].Patch, "+new") {
		t.Errorf("patch missing +new line:\n%s", files[0].Patch)
	}
}

func TestDiff_DeletedFile(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	doomed := filepath.Join(dir, "doomed.journal")
	if err := os.WriteFile(doomed, []byte("bye\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add doomed"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := os.Remove(doomed); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "delete doomed"); err != nil {
		t.Fatalf("Commit delete: %v", err)
	}

	snaps, _ := repo.List(ctx, 1)
	files, err := repo.Diff(ctx, snaps[0].Hash)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(files) != 1 || files[0].Path != "doomed.journal" || files[0].Change != ChangeDeleted {
		t.Fatalf("expected single ChangeDeleted for doomed.journal, got %+v", files)
	}
}

func TestDiff_RenamedFile(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	content := []byte("the quick brown fox jumps over the lazy dog\n" +
		"second line of identical content for rename detection\n")
	if err := os.WriteFile(filepath.Join(dir, "old.journal"), content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add old"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := os.Rename(filepath.Join(dir, "old.journal"), filepath.Join(dir, "new.journal")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "rename old to new"); err != nil {
		t.Fatalf("Commit rename: %v", err)
	}

	snaps, _ := repo.List(ctx, 1)
	files, err := repo.Diff(ctx, snaps[0].Hash)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d (%+v)", len(files), files)
	}
	if files[0].Change != ChangeRenamed {
		t.Errorf("change: got %v, want ChangeRenamed", files[0].Change)
	}
	if files[0].OldPath != "old.journal" || files[0].Path != "new.journal" {
		t.Errorf("paths: got old=%q new=%q, want old=old.journal new=new.journal", files[0].OldPath, files[0].Path)
	}
}

func TestDiff_RootCommit(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	snaps, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	rootHash := snaps[len(snaps)-1].Hash

	files, err := repo.Diff(ctx, rootHash)
	if err != nil {
		t.Fatalf("Diff(root): %v", err)
	}
	found := false
	for _, f := range files {
		if f.Path == ".gitignore" && f.Change == ChangeAdded {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected .gitignore as added file in root commit; got %+v", files)
	}
}

func TestDiff_InvalidHash(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := repo.Diff(ctx, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"); err == nil {
		t.Fatal("expected error for nonexistent hash, got nil")
	}
	if _, err := repo.Diff(ctx, ""); err == nil {
		t.Fatal("expected error for empty hash, got nil")
	}
}

func TestDiff_BinaryFile(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	binPath := filepath.Join(dir, "data.bin")
	if err := os.WriteFile(binPath, []byte{0, 1, 2, 0, 3}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "add binary"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := os.WriteFile(binPath, []byte{0, 1, 2, 0, 3, 4, 5}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "modify binary"); err != nil {
		t.Fatalf("Commit modify: %v", err)
	}

	snaps, _ := repo.List(ctx, 1)
	files, err := repo.Diff(ctx, snaps[0].Hash)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if !files[0].IsBinary {
		t.Errorf("expected IsBinary=true, got false")
	}
	if files[0].Patch != "" {
		t.Errorf("expected empty patch for binary, got %q", files[0].Patch)
	}
}

func TestRecoverUncommitted_CleanTree(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := repo.RecoverUncommitted(ctx); err != nil {
		t.Fatalf("RecoverUncommitted on clean tree: %v", err)
	}

	snaps, err := repo.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(snaps))
	}
}

// TestCommit_IgnoresSSHKeyMaterial guards against committing the SSH host key
// or known-hosts file into snapshot history.
func TestCommit_IgnoresSSHKeyMaterial(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"ssh_host_key":    "PRIVATE KEY MATERIAL",
		"ssh_known_hosts": "host key fingerprints",
		"main.journal":    "; journal\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	r, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.Commit(t.Context(), "test commit"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	head, err := r.repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	commit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("CommitObject: %v", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	for _, secret := range []string{"ssh_host_key", "ssh_known_hosts", "config.toml", "float.key"} {
		if _, err := tree.File(secret); err == nil {
			t.Errorf("%s was committed to the snapshot repo", secret)
		}
	}
	if _, err := tree.File("main.journal"); err != nil {
		t.Errorf("main.journal missing from commit: %v", err)
	}
}

// TestNew_SelfHealsGitignore verifies that repos initialized by older float
// versions (whose .gitignore lacks the SSH entries) are updated on open,
// preserving user-added lines.
func TestNew_SelfHealsGitignore(t *testing.T) {
	dir := t.TempDir()
	old := "config.toml\nfloat.key\nmy-custom-entry\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir); err != nil { // init path
		t.Fatalf("New (init): %v", err)
	}
	if _, err := New(dir); err != nil { // open path
		t.Fatalf("New (open): %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{"config.toml", "float.key", "ssh_host_key", "ssh_known_hosts", "my-custom-entry"} {
		if !strings.Contains(got, want) {
			t.Errorf(".gitignore missing %q:\n%s", want, got)
		}
	}
}
