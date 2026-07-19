package transmission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// assertPriorityIndices checks a torrent-set request assigns wantHigh contiguous
// high-priority indices [0..wantHigh) and wantLow low-priority indices following them.
func assertPriorityIndices(t *testing.T, args json.RawMessage, wantHigh, wantLow int) {
	t.Helper()
	var setArgs TorrentSetArgs
	if err := json.Unmarshal(args, &setArgs); err != nil {
		t.Fatalf("unmarshal torrent-set args: %v", err)
	}

	if len(setArgs.PriorityHigh) != wantHigh {
		t.Errorf("expected %d high-priority files, got %d", wantHigh, len(setArgs.PriorityHigh))
	}
	if len(setArgs.PriorityLow) != wantLow {
		t.Errorf("expected %d low-priority files, got %d", wantLow, len(setArgs.PriorityLow))
	}
	for i, idx := range setArgs.PriorityHigh {
		if idx != i {
			t.Errorf("PriorityHigh[%d] = %d, want %d", i, idx, i)
		}
	}
	for i, idx := range setArgs.PriorityLow {
		if idx != i+wantHigh {
			t.Errorf("PriorityLow[%d] = %d, want %d", i, idx, i+wantHigh)
		}
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

func TestSetHighPriorityFiles(t *testing.T) {
	var calls []RPCRequest
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")

		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req)

		switch req.Method {
		case "torrent-get":
			result := TorrentWithFilesResult{
				Torrents: []TorrentWithFiles{{
					ID: 1,
					Files: []TorrentFile{
						{Name: "ep01.mkv", Length: 100},
						{Name: "ep02.mkv", Length: 200},
						{Name: "ep03.mkv", Length: 300},
						{Name: "ep04.mkv", Length: 400},
						{Name: "ep05.mkv", Length: 500},
					},
				}},
			}
			args, _ := json.Marshal(result)
			json.NewEncoder(w).Encode(RPCResponse{Result: "success", Arguments: args})
		case "torrent-set":
			assertPriorityIndices(t, req.Arguments, 2, 3)
			json.NewEncoder(w).Encode(RPCResponse{Result: "success"})
		}
	}

	_, c := newTestServer(t, handler)
	err := c.SetHighPriorityFiles(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("SetHighPriorityFiles: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 RPC calls, got %d", len(calls))
	}
}

func TestSetHighPriorityFilesAllHigh(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")

		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		switch req.Method {
		case "torrent-get":
			result := TorrentWithFilesResult{
				Torrents: []TorrentWithFiles{{
					ID:    1,
					Files: []TorrentFile{{Name: "file1.mkv", Length: 100}},
				}},
			}
			args, _ := json.Marshal(result)
			json.NewEncoder(w).Encode(RPCResponse{Result: "success", Arguments: args})
		case "torrent-set":
			assertPriorityIndices(t, req.Arguments, 1, 0)
			json.NewEncoder(w).Encode(RPCResponse{Result: "success"})
		}
	}

	_, c := newTestServer(t, handler)
	err := c.SetHighPriorityFiles(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("SetHighPriorityFiles: %v", err)
	}
}

func TestGetTorrents(t *testing.T) {
	result := TorrentGetResult{Torrents: []Torrent{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}}
	_, c := newTestServer(t, rpcHandler(t, "torrent-get", result))

	torrents, err := c.GetTorrents(context.Background())
	if err != nil {
		t.Fatalf("GetTorrents: %v", err)
	}
	if len(torrents) != 2 {
		t.Fatalf("expected 2 torrents, got %d", len(torrents))
	}
}

func TestAddMagnet(t *testing.T) {
	result := TorrentAddResult{TorrentAdded: &TorrentAdded{ID: 5, Name: "magnet-torrent"}}
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		if req.Method != "torrent-add" {
			t.Fatalf("method = %q, want torrent-add", req.Method)
		}
		var args TorrentAddArgs
		json.Unmarshal(req.Arguments, &args) //nolint:errcheck
		if args.Filename != "magnet:?xt=urn:btih:abc" {
			t.Errorf("Filename = %q", args.Filename)
		}
		out, _ := json.Marshal(result)
		json.NewEncoder(w).Encode(RPCResponse{Result: "success", Arguments: out}) //nolint:errcheck
	}

	_, c := newTestServer(t, handler)
	added, err := c.AddMagnet(context.Background(), "magnet:?xt=urn:btih:abc")
	if err != nil {
		t.Fatalf("AddMagnet: %v", err)
	}
	if added.ID != 5 || added.Name != "magnet-torrent" {
		t.Errorf("added = %+v", added)
	}
}

func TestAddTorrentFilePaused(t *testing.T) {
	result := TorrentAddResult{TorrentAdded: &TorrentAdded{ID: 9}}
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		var args TorrentAddArgs
		json.Unmarshal(req.Arguments, &args) //nolint:errcheck
		if !args.Paused {
			t.Error("expected Paused to be true")
		}
		if args.Metainfo != "base64data" {
			t.Errorf("Metainfo = %q", args.Metainfo)
		}
		out, _ := json.Marshal(result)
		json.NewEncoder(w).Encode(RPCResponse{Result: "success", Arguments: out}) //nolint:errcheck
	}

	_, c := newTestServer(t, handler)
	if _, err := c.AddTorrentFilePaused(context.Background(), "base64data"); err != nil {
		t.Fatalf("AddTorrentFilePaused: %v", err)
	}
}

func TestAddTorrentDuplicate(t *testing.T) {
	result := TorrentAddResult{TorrentDuplicate: &TorrentAdded{ID: 3, Name: "dup"}}
	_, c := newTestServer(t, rpcHandler(t, "torrent-add", result))

	_, err := c.AddTorrentFile(context.Background(), "meta")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %v, want duplicate", err)
	}
}

func TestAddTorrentEmptyResult(t *testing.T) {
	_, c := newTestServer(t, rpcHandler(t, "torrent-add", TorrentAddResult{}))

	if _, err := c.AddTorrentFile(context.Background(), "meta"); err == nil {
		t.Fatal("expected error for empty torrent-add result")
	}
}

func TestSetFilesWanted(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		if req.Method != "torrent-set" {
			t.Fatalf("method = %q, want torrent-set", req.Method)
		}
		var args TorrentSetArgs
		json.Unmarshal(req.Arguments, &args) //nolint:errcheck
		if len(args.FilesWanted) != 2 || len(args.FilesUnwanted) != 1 {
			t.Errorf("wanted/unwanted = %v/%v", args.FilesWanted, args.FilesUnwanted)
		}
		json.NewEncoder(w).Encode(RPCResponse{Result: "success"}) //nolint:errcheck
	}

	_, c := newTestServer(t, handler)
	if err := c.SetFilesWanted(context.Background(), 1, []int{0, 1}, []int{2}); err != nil {
		t.Fatalf("SetFilesWanted: %v", err)
	}
}

func TestSetLabels(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		var args TorrentSetArgs
		json.Unmarshal(req.Arguments, &args) //nolint:errcheck
		if args.Labels == nil || len(*args.Labels) != 1 || (*args.Labels)[0] != "movies" {
			t.Errorf("labels = %v", args.Labels)
		}
		json.NewEncoder(w).Encode(RPCResponse{Result: "success"}) //nolint:errcheck
	}

	_, c := newTestServer(t, handler)
	if err := c.SetLabels(context.Background(), []int64{1}, []string{"movies"}); err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
}

func TestSetLabelsClearsWithNil(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		var args TorrentSetArgs
		json.Unmarshal(req.Arguments, &args) //nolint:errcheck
		if args.Labels == nil || len(*args.Labels) != 0 {
			t.Errorf("expected empty non-nil labels, got %v", args.Labels)
		}
		json.NewEncoder(w).Encode(RPCResponse{Result: "success"}) //nolint:errcheck
	}

	_, c := newTestServer(t, handler)
	if err := c.SetLabels(context.Background(), []int64{1}, nil); err != nil {
		t.Fatalf("SetLabels: %v", err)
	}
}

func TestGetTorrentFiles(t *testing.T) {
	result := TorrentWithFilesResult{
		Torrents: []TorrentWithFiles{{ID: 1, Files: []TorrentFile{{Name: "a"}, {Name: "b"}}}},
	}
	_, c := newTestServer(t, rpcHandler(t, "torrent-get", result))

	files, err := c.GetTorrentFiles(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetTorrentFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
}

func TestGetTorrentFilesNotFound(t *testing.T) {
	_, c := newTestServer(t, rpcHandler(t, "torrent-get", TorrentWithFilesResult{}))

	if _, err := c.GetTorrentFiles(context.Background(), 99); err == nil {
		t.Fatal("expected error for missing torrent")
	}
}

func TestSessionGet(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")
		var req RPCRequest
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		if req.Method != "session-get" {
			t.Fatalf("method = %q, want session-get", req.Method)
		}
		json.NewEncoder(w).Encode(RPCResponse{Result: "success", Arguments: json.RawMessage(`{"version":"4.0"}`)}) //nolint:errcheck
	}

	_, c := newTestServer(t, handler)
	args, err := c.SessionGet(context.Background())
	if err != nil {
		t.Fatalf("SessionGet: %v", err)
	}
	if !strings.Contains(string(args), "4.0") {
		t.Errorf("args = %s", args)
	}
}

func TestSessionGetFailure(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")
		json.NewEncoder(w).Encode(RPCResponse{Result: "failure"}) //nolint:errcheck
	}

	_, c := newTestServer(t, handler)
	if _, err := c.SessionGet(context.Background()); err == nil {
		t.Fatal("expected error on failure result")
	}
}

func TestDoRaw(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "test-session")
		w.Write([]byte(`{"result":"success","arguments":{"echo":true}}`)) //nolint:errcheck
	}

	_, c := newTestServer(t, handler)
	out, err := c.DoRaw(context.Background(), []byte(`{"method":"torrent-get"}`))
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	if !strings.Contains(string(out), "echo") {
		t.Errorf("out = %s", out)
	}
}

func TestDoRawSessionRefresh(t *testing.T) {
	var calls atomic.Int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(sessionIDHeader, "fresh")
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.Write([]byte(`{"result":"success"}`)) //nolint:errcheck
	}

	_, c := newTestServer(t, handler)
	if _, err := c.DoRaw(context.Background(), []byte(`{"method":"torrent-get"}`)); err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	// 3 requests: initial POST (409), GET to refresh the session id, then the retry POST.
	if calls.Load() != 3 {
		t.Fatalf("expected 3 requests (409, refresh, retry), got %d", calls.Load())
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
