package transmission

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

const sessionIDHeader = "X-Transmission-Session-Id"

// Client is an HTTP client for the Transmission RPC API.
type Client struct {
	url        string
	authHeader string
	sessionID  atomic.Value // stores string
	httpClient *http.Client
	group      singleflight.Group
}

// NewClient creates a new Transmission RPC client.
func NewClient(rpcURL, user, pass string) *Client {
	auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	c := &Client{
		url:        rpcURL,
		authHeader: "Basic " + auth,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	c.sessionID.Store("")
	return c
}

// Do executes a Transmission RPC request, handling session ID refresh on 409.
func (c *Client) Do(ctx context.Context, req RPCRequest) (RPCResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return RPCResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	resp, sessionID, err := c.doHTTP(ctx, body)
	if err != nil {
		return RPCResponse{}, err
	}

	if resp.StatusCode == http.StatusConflict {
		newID, err := c.refreshSessionID(ctx, sessionID)
		if err != nil {
			return RPCResponse{}, fmt.Errorf("refresh session id: %w", err)
		}
		resp.Body.Close()
		resp, err = c.doHTTPWithSession(ctx, body, newID)
		if err != nil {
			return RPCResponse{}, err
		}
	}

	defer resp.Body.Close()
	var rpcResp RPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return RPCResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return rpcResp, nil
}

func (c *Client) doHTTP(ctx context.Context, body []byte) (*http.Response, string, error) {
	sessionID := c.sessionID.Load().(string)
	resp, err := c.doHTTPWithSession(ctx, body, sessionID)
	return resp, sessionID, err
}

func (c *Client) doHTTPWithSession(ctx context.Context, body []byte, sessionID string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader)
	if sessionID != "" {
		req.Header.Set(sessionIDHeader, sessionID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	return resp, nil
}

// refreshSessionID fetches a new session ID using singleflight to avoid thundering herd.
func (c *Client) refreshSessionID(ctx context.Context, staleID string) (string, error) {
	v, err, _ := c.group.Do("session-refresh", func() (any, error) {
		// Double-check: another goroutine may have already refreshed it.
		if current := c.sessionID.Load().(string); current != staleID {
			return current, nil
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
		if err != nil {
			return "", fmt.Errorf("create session request: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("fetch session id: %w", err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body) //nolint:errcheck

		newID := resp.Header.Get(sessionIDHeader)
		if newID == "" {
			return "", fmt.Errorf("empty session id in response")
		}
		c.sessionID.Store(newID)
		slog.Debug("session id refreshed", "id", newID)
		return newID, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// DoRaw forwards a raw JSON body to Transmission and returns the raw response body.
// Used by the HTTP proxy to avoid double-encoding.
func (c *Client) DoRaw(ctx context.Context, body []byte) ([]byte, error) {
	resp, sessionID, err := c.doHTTP(ctx, body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusConflict {
		newID, err := c.refreshSessionID(ctx, sessionID)
		if err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("refresh session id: %w", err)
		}
		resp.Body.Close()
		resp, err = c.doHTTPWithSession(ctx, body, newID)
		if err != nil {
			return nil, err
		}
	}

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return data, nil
}

// GetTorrents returns the full torrent list.
func (c *Client) GetTorrents(ctx context.Context) ([]Torrent, error) {
	args, err := json.Marshal(TorrentGetArgs{Fields: TorrentFields})
	if err != nil {
		return nil, err
	}

	resp, err := c.Do(ctx, RPCRequest{Method: "torrent-get", Arguments: args})
	if err != nil {
		return nil, err
	}
	if resp.Result != "success" {
		return nil, fmt.Errorf("torrent-get failed: %s", resp.Result)
	}

	var result TorrentGetResult
	if err := json.Unmarshal(resp.Arguments, &result); err != nil {
		return nil, fmt.Errorf("unmarshal torrents: %w", err)
	}
	return result.Torrents, nil
}

// AddMagnet adds a torrent by magnet link or URL.
func (c *Client) AddMagnet(ctx context.Context, magnet string) (TorrentAdded, error) {
	return c.addTorrent(ctx, TorrentAddArgs{Filename: magnet})
}

// AddTorrentFile adds a torrent from a base64-encoded .torrent file.
func (c *Client) AddTorrentFile(ctx context.Context, metainfo string) (TorrentAdded, error) {
	return c.addTorrent(ctx, TorrentAddArgs{Metainfo: metainfo})
}

func (c *Client) addTorrent(ctx context.Context, addArgs TorrentAddArgs) (TorrentAdded, error) {
	args, err := json.Marshal(addArgs)
	if err != nil {
		return TorrentAdded{}, err
	}

	resp, err := c.Do(ctx, RPCRequest{Method: "torrent-add", Arguments: args})
	if err != nil {
		return TorrentAdded{}, err
	}
	if resp.Result != "success" {
		return TorrentAdded{}, fmt.Errorf("torrent-add failed: %s", resp.Result)
	}

	var result TorrentAddResult
	if err := json.Unmarshal(resp.Arguments, &result); err != nil {
		return TorrentAdded{}, fmt.Errorf("unmarshal add result: %w", err)
	}

	if result.TorrentAdded != nil {
		return *result.TorrentAdded, nil
	}
	if result.TorrentDuplicate != nil {
		return *result.TorrentDuplicate, fmt.Errorf("duplicate torrent: %s", result.TorrentDuplicate.Name)
	}
	return TorrentAdded{}, fmt.Errorf("unexpected empty torrent-add result")
}

// SessionGet calls session-get and returns the raw arguments.
func (c *Client) SessionGet(ctx context.Context) (json.RawMessage, error) {
	resp, err := c.Do(ctx, RPCRequest{Method: "session-get"})
	if err != nil {
		return nil, err
	}
	if resp.Result != "success" {
		return nil, fmt.Errorf("session-get failed: %s", resp.Result)
	}
	return resp.Arguments, nil
}
