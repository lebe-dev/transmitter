package movie

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStoreFlagsSurviveMatchChanges(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	if err := store.SetFlags(ctx, hashA, Flags{Watched: ptr(true), MissingSubs: ptr(true)}); err != nil {
		t.Fatalf("SetFlags() error = %v", err)
	}
	rec, ok, _ := store.Get(ctx, hashA)
	if !ok || rec.State != "" || !rec.Watched || !rec.MissingSubs {
		t.Fatalf("record = %+v, want marks without a lookup state", rec)
	}

	store.SaveMatch(ctx, hashA, interstellar, false, true, "q") //nolint:errcheck
	store.SetFlags(ctx, hashA, Flags{MissingSubs: ptr(false)})  //nolint:errcheck
	rec, _, _ = store.Get(ctx, hashA)
	if !rec.Watched || rec.MissingSubs || rec.Title != interstellar.Title || !rec.HasPoster {
		t.Errorf("record = %+v, want the match with watched kept and subs cleared", rec)
	}

	store.SaveState(ctx, hashA, StateUnmatched, "", true) //nolint:errcheck
	rec, _, _ = store.Get(ctx, hashA)
	if !rec.Watched || rec.Title != "" || rec.HasPoster {
		t.Errorf("record = %+v, want the match cleared and the marks kept", rec)
	}
}

func TestStoreRenameIsForgottenForAnotherTitle(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	store.SaveMatch(ctx, hashA, interstellar, false, true, "") //nolint:errcheck
	store.SetRenamed(ctx, hashA, "Interstellar (2014)")        //nolint:errcheck

	store.SaveMatch(ctx, hashA, interstellar, true, true, "") //nolint:errcheck
	if rec, _, _ := store.Get(ctx, hashA); rec.RenamedTo == "" {
		t.Error("re-saving the same title must keep the rename")
	}

	other := interstellar
	other.TMDBID = 1
	store.SaveMatch(ctx, hashA, other, true, true, "") //nolint:errcheck
	if rec, _, _ := store.Get(ctx, hashA); rec.RenamedTo != "" {
		t.Error("a different title must reset the rename")
	}
}

func TestStoreFailedAttemptsAccumulate(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()

	store.SaveState(ctx, hashA, StateFailed, "q", false) //nolint:errcheck
	store.SaveState(ctx, hashA, StateFailed, "q", false) //nolint:errcheck
	if rec, _, _ := store.Get(ctx, hashA); rec.Attempts != 2 {
		t.Errorf("attempts = %d, want 2", rec.Attempts)
	}
	store.SaveState(ctx, hashA, StateUnmatched, "q", false) //nolint:errcheck
	if rec, _, _ := store.Get(ctx, hashA); rec.Attempts != 0 {
		t.Errorf("attempts = %d, want reset", rec.Attempts)
	}
}

func TestStoreDeleteExcept(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	store.SetFlags(ctx, hashA, Flags{Watched: ptr(true)}) //nolint:errcheck
	store.SetFlags(ctx, hashB, Flags{Watched: ptr(true)}) //nolint:errcheck

	deleted, err := store.DeleteExcept(ctx, []string{hashB})
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteExcept() = %d, %v", deleted, err)
	}
	all, _ := store.All(ctx)
	if _, ok := all[hashB]; !ok || len(all) != 1 {
		t.Errorf("All() = %v, want only hashB", all)
	}
}

func TestStoreSinceIsStable(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	first, err := store.Since(ctx)
	if err != nil {
		t.Fatalf("Since() error = %v", err)
	}
	store.now = func() time.Time { return time.Now().Add(time.Hour) }
	second, _ := store.Since(ctx)
	if !first.Equal(second) {
		t.Errorf("Since() moved from %v to %v", first, second)
	}
}

func TestStoreRejectsInvalidHash(t *testing.T) {
	store := newStore(t)
	if err := store.SetFlags(context.Background(), "nope", Flags{}); !errors.Is(err, ErrInvalidHash) {
		t.Errorf("SetFlags() error = %v, want ErrInvalidHash", err)
	}
}
