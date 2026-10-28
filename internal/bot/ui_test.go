package bot

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/transmitter/internal/transmission"
	"gopkg.in/telebot.v4"
)

func countButtons(kb [][]telebot.InlineButton) int {
	n := 0
	for _, row := range kb {
		n += len(row)
	}
	return n
}

func TestParseID(t *testing.T) {
	cases := []struct {
		data, prefix string
		want         int64
		wantErr      bool
	}{
		{"d:42", "d:", 42, false},
		{"da:7", "da:", 7, false},
		{"x:0", "x:", 0, false},
		{"d:abc", "d:", 0, true},
		{"d:", "d:", 0, true},
	}
	for _, tc := range cases {
		got, err := parseID(tc.data, tc.prefix)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseID(%q,%q) expected error", tc.data, tc.prefix)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseID(%q,%q): %v", tc.data, tc.prefix, err)
		}
		if got != tc.want {
			t.Errorf("parseID(%q,%q) = %d, want %d", tc.data, tc.prefix, got, tc.want)
		}
	}
}

func TestFilterActive(t *testing.T) {
	in := []transmission.Torrent{
		{ID: 1, Status: 0},
		{ID: 2, Status: 4},
		{ID: 3, Status: 0},
		{ID: 4, Status: 6},
	}
	got := filterActive(in)
	if len(got) != 2 {
		t.Fatalf("filterActive returned %d, want 2", len(got))
	}
	if got[0].ID != 2 || got[1].ID != 4 {
		t.Errorf("filterActive kept wrong torrents: %+v", got)
	}
}

func TestFilterActiveEmpty(t *testing.T) {
	if got := filterActive(nil); len(got) != 0 {
		t.Errorf("filterActive(nil) = %v, want empty", got)
	}
}

func TestDetailKeyboardPauseResume(t *testing.T) {
	paused := detailKeyboard(transmission.Torrent{ID: 5, Status: 0}, 0, false)
	if !hasButtonUnique(paused.InlineKeyboard, "r:5") {
		t.Error("expected Resume button for stopped torrent")
	}

	active := detailKeyboard(transmission.Torrent{ID: 5, Status: 4}, 0, false)
	if !hasButtonUnique(active.InlineKeyboard, "p:5") {
		t.Error("expected Pause button for active torrent")
	}
}

func TestDetailKeyboardBackPrefix(t *testing.T) {
	all := detailKeyboard(transmission.Torrent{ID: 5, Status: 4}, 2, true)
	if !hasButtonUnique(all.InlineKeyboard, "ba:2") {
		t.Error("expected all-torrents back prefix ba:")
	}
	active := detailKeyboard(transmission.Torrent{ID: 5, Status: 4}, 2, false)
	if !hasButtonUnique(active.InlineKeyboard, "b:2") {
		t.Error("expected active back prefix b:")
	}
}

func TestDeleteConfirmKeyboard(t *testing.T) {
	kb := deleteConfirmKeyboard(9)
	for _, want := range []string{"xk:9", "xd:9", "c"} {
		if !hasButtonUnique(kb.InlineKeyboard, want) {
			t.Errorf("expected button with data %q", want)
		}
	}
}

func TestStatusPageKeyboardPagination(t *testing.T) {
	// Build more torrents than fit on one page to force nav buttons.
	var torrents []transmission.Torrent
	for i := 1; i <= torrentsPerPage*2; i++ {
		torrents = append(torrents, transmission.Torrent{ID: int64(i), Name: "t", Status: 4})
	}

	kb := statusPageKeyboard(torrents, 0, false)
	if countButtons(kb.InlineKeyboard) == 0 {
		t.Fatal("expected a non-empty keyboard")
	}
	if !hasButtonUnique(kb.InlineKeyboard, "s:1") {
		t.Error("expected Next button targeting page 1 (prefix s:)")
	}

	kbAll := statusPageKeyboard(torrents, 0, true)
	if !hasButtonUnique(kbAll.InlineKeyboard, "sa:1") {
		t.Error("expected Next button with all-torrents prefix sa:")
	}
}

// hasButtonUnique reports whether any inline button carries the exact Data value.
func hasButtonUnique(kb [][]telebot.InlineButton, data string) bool {
	for _, row := range kb {
		for _, btn := range row {
			if btn.Data == data {
				return true
			}
		}
	}
	return false
}

func TestFileSelectStateLifecycle(t *testing.T) {
	b := &Bot{
		logger:     nopLogger(),
		fileSelect: make(map[int64]*FileSelectState),
	}

	if b.getFileSelectState(1) != nil {
		t.Fatal("expected nil for unknown state")
	}

	state := &FileSelectState{TorrentID: 1, CreatedAt: time.Now()}
	b.setFileSelectState(1, state)
	if got := b.getFileSelectState(1); got != state {
		t.Fatal("setFileSelectState did not store the state")
	}

	b.deleteFileSelectState(1)
	if b.getFileSelectState(1) != nil {
		t.Fatal("deleteFileSelectState did not remove the state")
	}
}

func TestCleanupStaleFileSelectStates(t *testing.T) {
	var mu sync.Mutex
	var removed []int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Transmission-Session-Id", "s")
		mu.Lock()
		removed = append(removed, 1)
		mu.Unlock()
		w.Write([]byte(`{"result":"success"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	b := &Bot{
		logger:            nopLogger(),
		client:            transmission.NewClient(srv.URL, "u", "p"),
		fileSelect:        make(map[int64]*FileSelectState),
		fileSelectTimeout: time.Minute,
	}

	// One stale (created 2h ago), one fresh.
	b.fileSelect[1] = &FileSelectState{TorrentID: 1, CreatedAt: time.Now().Add(-2 * time.Hour)}
	b.fileSelect[2] = &FileSelectState{TorrentID: 2, CreatedAt: time.Now()}

	b.cleanupStaleFileSelectStates()

	if b.getFileSelectState(1) != nil {
		t.Error("stale state should have been removed")
	}
	if b.getFileSelectState(2) == nil {
		t.Error("fresh state should have been kept")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(removed) != 1 {
		t.Errorf("expected 1 torrent-remove call, got %d", len(removed))
	}
}
