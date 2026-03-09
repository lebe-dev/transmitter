package transmission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, "user", "pass")
	return srv, c
}

func rpcHandler(t *testing.T, wantMethod string, result any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")

		var req RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != wantMethod {
			t.Fatalf("unexpected method: got %q, want %q", req.Method, wantMethod)
		}

		args, _ := json.Marshal(result)
		resp := RPCResponse{Result: "success", Arguments: args}
		json.NewEncoder(w).Encode(resp)
	}
}

func TestGetTorrent(t *testing.T) {
	result := TorrentGetResult{
		Torrents: []Torrent{{ID: 42, Name: "test-torrent", PercentDone: 0.75}},
	}
	_, c := newTestServer(t, rpcHandler(t, "torrent-get", result))

	torrent, err := c.GetTorrent(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetTorrent: %v", err)
	}
	if torrent.ID != 42 {
		t.Errorf("ID = %d, want 42", torrent.ID)
	}
	if torrent.Name != "test-torrent" {
		t.Errorf("Name = %q, want %q", torrent.Name, "test-torrent")
	}
}

func TestGetTorrentNotFound(t *testing.T) {
	result := TorrentGetResult{Torrents: []Torrent{}}
	_, c := newTestServer(t, rpcHandler(t, "torrent-get", result))

	_, err := c.GetTorrent(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for missing torrent")
	}
}

func TestStartTorrents(t *testing.T) {
	_, c := newTestServer(t, rpcHandler(t, "torrent-start", nil))
	if err := c.StartTorrents(context.Background(), []int64{1, 2}); err != nil {
		t.Fatalf("StartTorrents: %v", err)
	}
}

func TestStopTorrents(t *testing.T) {
	_, c := newTestServer(t, rpcHandler(t, "torrent-stop", nil))
	if err := c.StopTorrents(context.Background(), []int64{1}); err != nil {
		t.Fatalf("StopTorrents: %v", err)
	}
}

func TestRemoveTorrents(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")

		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		var args TorrentActionArgs
		json.Unmarshal(req.Arguments, &args)

		if !args.DeleteLocalData {
			t.Error("expected DeleteLocalData to be true")
		}

		resp := RPCResponse{Result: "success"}
		json.NewEncoder(w).Encode(resp)
	}

	_, c := newTestServer(t, handler)
	if err := c.RemoveTorrents(context.Background(), []int64{1}, true); err != nil {
		t.Fatalf("RemoveTorrents: %v", err)
	}
}

func TestRemoveTorrentsKeepData(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")

		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		var args TorrentActionArgs
		json.Unmarshal(req.Arguments, &args)

		if args.DeleteLocalData {
			t.Error("expected DeleteLocalData to be false")
		}

		resp := RPCResponse{Result: "success"}
		json.NewEncoder(w).Encode(resp)
	}

	_, c := newTestServer(t, handler)
	if err := c.RemoveTorrents(context.Background(), []int64{1}, false); err != nil {
		t.Fatalf("RemoveTorrents: %v", err)
	}
}

func TestSessionIDRefresh(t *testing.T) {
	var calls atomic.Int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set(sessionIDHeader, "new-session")
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.Header().Set(sessionIDHeader, "new-session")
		result := TorrentGetResult{
			Torrents: []Torrent{{ID: 1, Name: "t1"}},
		}
		args, _ := json.Marshal(result)
		json.NewEncoder(w).Encode(RPCResponse{Result: "success", Arguments: args})
	}

	_, c := newTestServer(t, handler)
	torrents, err := c.GetTorrents(context.Background())
	if err != nil {
		t.Fatalf("GetTorrents: %v", err)
	}
	if len(torrents) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(torrents))
	}
}
