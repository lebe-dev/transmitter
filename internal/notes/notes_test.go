package notes

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func newStore(t *testing.T, maxLen int) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "notes.db"), maxLen)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { store.Close() }) //nolint:errcheck
	return store
}

func TestSetAndGet(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	if err := store.Set(ctx, hashA, "смотреть с семьёй"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := store.Get(ctx, hashA)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "смотреть с семьёй" {
		t.Errorf("Get() = %q, want %q", got, "смотреть с семьёй")
	}
}

func TestGetMissingReturnsEmpty(t *testing.T) {
	store := newStore(t, 200)

	got, err := store.Get(context.Background(), hashA)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != "" {
		t.Errorf("Get() = %q, want empty string", got)
	}
}

func TestSetOverwritesExistingNote(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	if err := store.Set(ctx, hashA, "first"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Set(ctx, hashA, "second"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, _ := store.Get(ctx, hashA)
	if got != "second" {
		t.Errorf("Get() = %q, want %q", got, "second")
	}
}

func TestSetEmptyTextDeletesNote(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	if err := store.Set(ctx, hashA, "note"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Set(ctx, hashA, "   "); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	all, err := store.All(ctx)
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(all) != 0 {
		t.Errorf("All() = %v, want empty map", all)
	}
}

func TestSetRejectsTooLongNote(t *testing.T) {
	store := newStore(t, 10)
	ctx := context.Background()

	// Rune-based limit: 11 Cyrillic runes must be rejected even though the
	// byte length is far larger.
	err := store.Set(ctx, hashA, strings.Repeat("я", 11))
	if !errors.Is(err, ErrTooLong) {
		t.Fatalf("Set() error = %v, want ErrTooLong", err)
	}

	if err := store.Set(ctx, hashA, strings.Repeat("я", 10)); err != nil {
		t.Errorf("Set() with exactly maxLen runes error = %v, want nil", err)
	}
}

func TestSetRejectsInvalidHash(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	tests := []string{
		"",
		"short",
		strings.Repeat("z", 40),
		strings.Repeat("a", 65),
		"'; DROP TABLE torrent_notes; --",
	}
	for _, hash := range tests {
		if err := store.Set(ctx, hash, "note"); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("Set(%q) error = %v, want ErrInvalidHash", hash, err)
		}
	}
}

func TestHashIsCaseInsensitive(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	if err := store.Set(ctx, strings.ToUpper(hashA), "note"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, _ := store.Get(ctx, hashA)
	if got != "note" {
		t.Errorf("Get() = %q, want %q", got, "note")
	}
}

func TestSetStripsControlCharacters(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	if err := store.Set(ctx, hashA, "  line\x00one\r\nline two\x07  "); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, _ := store.Get(ctx, hashA)
	if got != "lineone\nline two" {
		t.Errorf("Get() = %q, want %q", got, "lineone\nline two")
	}
}

func TestAllReturnsEveryNote(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	store.Set(ctx, hashA, "a") //nolint:errcheck
	store.Set(ctx, hashB, "b") //nolint:errcheck

	all, err := store.All(ctx)
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(all) != 2 || all[hashA] != "a" || all[hashB] != "b" {
		t.Errorf("All() = %v, want map with 2 notes", all)
	}
}

func TestDeleteRemovesOnlyGivenHashes(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	store.Set(ctx, hashA, "a") //nolint:errcheck
	store.Set(ctx, hashB, "b") //nolint:errcheck

	if err := store.Delete(ctx, hashA); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	all, _ := store.All(ctx)
	if _, ok := all[hashA]; ok {
		t.Error("Delete() left the note of hashA in place")
	}
	if all[hashB] != "b" {
		t.Error("Delete() removed the note of an unrelated torrent")
	}
}

func TestDeleteWithoutHashesIsNoop(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	store.Set(ctx, hashA, "a") //nolint:errcheck

	if err := store.Delete(ctx); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	all, _ := store.All(ctx)
	if len(all) != 1 {
		t.Errorf("Delete() with no hashes changed the store: %v", all)
	}
}

func TestDeleteExceptDropsOrphans(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	store.Set(ctx, hashA, "a") //nolint:errcheck
	store.Set(ctx, hashB, "b") //nolint:errcheck

	deleted, err := store.DeleteExcept(ctx, []string{hashB})
	if err != nil {
		t.Fatalf("DeleteExcept() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("DeleteExcept() = %d, want 1", deleted)
	}

	all, _ := store.All(ctx)
	if len(all) != 1 || all[hashB] != "b" {
		t.Errorf("All() = %v, want only the note of hashB", all)
	}
}

func TestDeleteExceptWithEmptyKeepListWipesAll(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	store.Set(ctx, hashA, "a") //nolint:errcheck

	deleted, err := store.DeleteExcept(ctx, nil)
	if err != nil {
		t.Fatalf("DeleteExcept() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("DeleteExcept() = %d, want 1", deleted)
	}
}

func TestDeleteExceptRejectsInvalidHash(t *testing.T) {
	store := newStore(t, 200)
	ctx := context.Background()

	store.Set(ctx, hashA, "a") //nolint:errcheck

	if _, err := store.DeleteExcept(ctx, []string{"bogus"}); !errors.Is(err, ErrInvalidHash) {
		t.Fatalf("DeleteExcept() error = %v, want ErrInvalidHash", err)
	}

	all, _ := store.All(ctx)
	if len(all) != 1 {
		t.Error("DeleteExcept() deleted notes despite an invalid keep list")
	}
}

func TestOpenCreatesMissingDirectoryAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "notes.db")
	ctx := context.Background()

	store, err := Open(path, 200)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Set(ctx, hashA, "persisted"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(path, 200)
	if err != nil {
		t.Fatalf("Open() after close error = %v", err)
	}
	defer reopened.Close() //nolint:errcheck

	got, _ := reopened.Get(ctx, hashA)
	if got != "persisted" {
		t.Errorf("Get() after reopen = %q, want %q", got, "persisted")
	}
}

func TestOpenFallsBackToDefaultMaxLength(t *testing.T) {
	store := newStore(t, 0)

	if store.MaxLength() != DefaultMaxLength {
		t.Errorf("MaxLength() = %d, want %d", store.MaxLength(), DefaultMaxLength)
	}
}
