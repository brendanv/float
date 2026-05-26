package gitsnap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gogitmem "github.com/go-git/go-git/v5/storage/memory"
	gogitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	gogitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
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

func TestRestore_PreservesIgnoredFiles(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	journalPath := filepath.Join(dir, "main.journal")
	configPath := filepath.Join(dir, "config.toml")

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
	if err := os.WriteFile(configPath, []byte("[server]\nport=8080\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "commit B"); err != nil {
		t.Fatalf("Commit B: %v", err)
	}

	if err := repo.Restore(ctx, hashA); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config.toml deleted by restore: %v", err)
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

// installInMemRemote installs a custom transport scheme backed by a memory
// storage so tests can push/fetch without the git binary or network.
// Returns the remote URL and the underlying storage for post-push inspection.
// Registers a t.Cleanup to restore the previous protocol state.
func installInMemRemote(t *testing.T) (remoteURL string, storer *gogitmem.Storage) {
	t.Helper()
	const scheme = "inmemtest"
	const url = scheme + "://float-test"
	storer = gogitmem.NewStorage()
	// Initialize as a bare repo so the receive-pack server can store refs.
	if _, err := gogit.Init(storer, nil); err != nil {
		t.Fatalf("init in-memory remote: %v", err)
	}
	loader := gogitserver.MapLoader{url: storer}
	old := gogitclient.Protocols[scheme]
	gogitclient.InstallProtocol(scheme, gogitserver.NewClient(loader))
	t.Cleanup(func() {
		if old == nil {
			delete(gogitclient.Protocols, scheme)
		} else {
			gogitclient.InstallProtocol(scheme, old)
		}
	})
	return url, storer
}

// waitForRemoteCommit polls the remote storer until it has at least one commit
// or the deadline passes.
func waitForRemoteCommit(t *testing.T, storer *gogitmem.Storage) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, err := gogit.Open(storer, nil)
		if err == nil {
			if _, err := r.Head(); err == nil {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for push to remote")
}

func TestSetRemote_PushAfterCommit(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	remoteURL, storer := installInMemRemote(t)
	if err := repo.SetRemote(remoteURL, nil); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Cleanup(repo.Close)

	if err := os.WriteFile(filepath.Join(dir, "test.journal"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "test push"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Wait for the background goroutine to complete the push.
	waitForRemoteCommit(t, storer)

	snaps, _ := repo.List(ctx, 1)
	r, _ := gogit.Open(storer, nil)
	head, _ := r.Head()
	if head.Hash().String() != snaps[0].Hash {
		t.Errorf("remote HEAD %s != local HEAD %s", head.Hash(), snaps[0].Hash)
	}
}

func TestSetRemote_NoPushOnCleanTree(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	remoteURL, _ := installInMemRemote(t)
	if err := repo.SetRemote(remoteURL, nil); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Cleanup(repo.Close)

	// Commit on clean tree is a no-op; no push should be enqueued.
	if err := repo.Commit(ctx, "no-op"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Channel should be empty.
	select {
	case <-repo.pushCh:
		t.Error("push was enqueued for a clean-tree no-op commit")
	default:
	}
}

func TestSetRemote_PushAfterRestore(t *testing.T) {
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
	if err := repo.Commit(ctx, "A"); err != nil {
		t.Fatalf("Commit A: %v", err)
	}
	snapsA, _ := repo.List(ctx, 1)
	hashA := snapsA[0].Hash

	if err := os.WriteFile(journalPath, []byte("version-B"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(ctx, "B"); err != nil {
		t.Fatalf("Commit B: %v", err)
	}

	remoteURL, storer := installInMemRemote(t)
	if err := repo.SetRemote(remoteURL, nil); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Cleanup(repo.Close)

	if err := repo.Restore(ctx, hashA); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Restore enqueues a push with Force:true so remote reflects the reverted HEAD.
	waitForRemoteCommit(t, storer)

	r, _ := gogit.Open(storer, nil)
	head, _ := r.Head()
	if head.Hash().String() != hashA {
		t.Errorf("remote HEAD %s != restored hash %s", head.Hash(), hashA)
	}
}

func TestClose_Idempotent(t *testing.T) {
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	remoteURL, _ := installInMemRemote(t)
	if err := repo.SetRemote(remoteURL, nil); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}

	// Should not panic when called multiple times.
	repo.Close()
	repo.Close()
}

func TestClose_NoRemote(t *testing.T) {
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Close without SetRemote must be a no-op.
	repo.Close()
}
