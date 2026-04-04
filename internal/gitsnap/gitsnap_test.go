package gitsnap

import (
	"os"
	"path/filepath"
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

func TestRestore_RevertsFiles(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	journalPath := filepath.Join(dir, "main.journal")

	if err := os.WriteFile(journalPath, []byte("version-A"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "commit A"); err != nil {
		t.Fatalf("Commit A: %v", err)
	}

	snapsA, _ := repo.List(ctx, 1)
	hashA := snapsA[0].Hash

	if err := os.WriteFile(journalPath, []byte("version-B"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "commit B"); err != nil {
		t.Fatalf("Commit B: %v", err)
	}

	if err := repo.Restore(ctx, hashA); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	got, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "version-A" {
		t.Errorf("after restore: got %q, want %q", got, "version-A")
	}
}

func TestRestore_InvalidHash(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = repo.Restore(ctx, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err == nil {
		t.Fatal("expected error for nonexistent hash, got nil")
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
