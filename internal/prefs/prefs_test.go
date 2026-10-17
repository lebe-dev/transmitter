package prefs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

const nightShift = "night-shift"

func newStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { store.Close() }) //nolint:errcheck
	return store
}

func TestShiftEnabledByDefault(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "prefs.db"))

	enabled, err := store.ShiftEnabled(context.Background(), nightShift)
	if err != nil {
		t.Fatalf("ShiftEnabled() error = %v", err)
	}
	if !enabled {
		t.Error("ShiftEnabled() = false, want true for a shift without a stored preference")
	}
}

func TestSetShiftEnabled(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "prefs.db"))
	ctx := context.Background()

	for _, want := range []bool{false, true, false} {
		if err := store.SetShiftEnabled(ctx, nightShift, want); err != nil {
			t.Fatalf("SetShiftEnabled(%v) error = %v", want, err)
		}
		got, err := store.ShiftEnabled(ctx, nightShift)
		if err != nil {
			t.Fatalf("ShiftEnabled() error = %v", err)
		}
		if got != want {
			t.Errorf("ShiftEnabled() = %v, want %v", got, want)
		}
	}
}

func TestShiftsAreIndependent(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "prefs.db"))
	ctx := context.Background()

	if err := store.SetShiftEnabled(ctx, nightShift, false); err != nil {
		t.Fatalf("SetShiftEnabled() error = %v", err)
	}

	enabled, err := store.ShiftEnabled(ctx, "day-shift")
	if err != nil {
		t.Fatalf("ShiftEnabled() error = %v", err)
	}
	if !enabled {
		t.Error("switching off one shift must not affect the other")
	}
}

// The toggle is set from the UI and has to survive a restart, which is why it
// lives in the database instead of an env var.
func TestShiftEnabledSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.db")
	ctx := context.Background()

	store := newStore(t, path)
	if err := store.SetShiftEnabled(ctx, nightShift, false); err != nil {
		t.Fatalf("SetShiftEnabled() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened := newStore(t, path)
	enabled, err := reopened.ShiftEnabled(ctx, nightShift)
	if err != nil {
		t.Fatalf("ShiftEnabled() error = %v", err)
	}
	if enabled {
		t.Error("ShiftEnabled() = true after reopen, want the stored false")
	}
}

func TestInvalidShiftName(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "prefs.db"))
	ctx := context.Background()

	if _, err := store.ShiftEnabled(ctx, "  "); !errors.Is(err, ErrInvalidShift) {
		t.Errorf("ShiftEnabled(blank) error = %v, want ErrInvalidShift", err)
	}
	if err := store.SetShiftEnabled(ctx, "", true); !errors.Is(err, ErrInvalidShift) {
		t.Errorf("SetShiftEnabled(empty) error = %v, want ErrInvalidShift", err)
	}
}

func TestImpactAutoStopDisabledByDefault(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "prefs.db"))

	enabled, err := store.ImpactAutoStopEnabled(context.Background())
	if err != nil {
		t.Fatalf("ImpactAutoStopEnabled() error = %v", err)
	}
	if enabled {
		t.Error("ImpactAutoStopEnabled() = true, want false until the user switches it on")
	}
}

func TestSetImpactAutoStopEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.db")
	ctx := context.Background()

	store := newStore(t, path)
	if err := store.SetImpactAutoStopEnabled(ctx, true); err != nil {
		t.Fatalf("SetImpactAutoStopEnabled() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened := newStore(t, path)
	enabled, err := reopened.ImpactAutoStopEnabled(ctx)
	if err != nil {
		t.Fatalf("ImpactAutoStopEnabled() error = %v", err)
	}
	if !enabled {
		t.Error("ImpactAutoStopEnabled() = false after reopen, want the stored true")
	}
}

func TestMemo(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "prefs.db"))
	ctx := context.Background()

	if got, err := store.Memo(ctx); err != nil || got != "" {
		t.Fatalf("Memo() = %q, %v; want empty", got, err)
	}
	for _, want := range []string{"line 1\nline 2\n", ""} {
		if err := store.SetMemo(ctx, want); err != nil {
			t.Fatalf("SetMemo(%q) error = %v", want, err)
		}
		got, err := store.Memo(ctx)
		if err != nil {
			t.Fatalf("Memo() error = %v", err)
		}
		if got != want {
			t.Errorf("Memo() = %q, want %q", got, want)
		}
	}
}

func TestPauseState(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "prefs.db"))
	ctx := context.Background()

	paused, hashes, err := store.PauseState(ctx)
	if err != nil || paused || hashes != nil {
		t.Fatalf("PauseState() = %v, %v, %v; want not paused", paused, hashes, err)
	}

	if err := store.SetPauseState(ctx, true, []string{"aa", "bb"}); err != nil {
		t.Fatalf("SetPauseState() error = %v", err)
	}
	paused, hashes, err = store.PauseState(ctx)
	if err != nil || !paused || len(hashes) != 2 || hashes[0] != "aa" || hashes[1] != "bb" {
		t.Fatalf("PauseState() = %v, %v, %v; want paused with [aa bb]", paused, hashes, err)
	}

	if err := store.SetPauseState(ctx, false, nil); err != nil {
		t.Fatalf("SetPauseState(false) error = %v", err)
	}
	paused, hashes, err = store.PauseState(ctx)
	if err != nil || paused || hashes != nil {
		t.Fatalf("PauseState() = %v, %v, %v; want cleared", paused, hashes, err)
	}
}
