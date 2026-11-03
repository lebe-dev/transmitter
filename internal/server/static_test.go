package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStaticHandlerCachingAndEncoding(t *testing.T) {
	fsys := fstest.MapFS{
		"dist/index.html":                     {Data: []byte("<html>index</html>")},
		"dist/index.html.br":                  {Data: []byte("br-index")},
		"dist/index.html.gz":                  {Data: []byte("gz-index")},
		"dist/_app/immutable/entry/app.js":    {Data: []byte("console.log(1)")},
		"dist/_app/immutable/entry/app.js.gz": {Data: []byte("gz-app")},
		"dist/favicon.png":                    {Data: []byte("png")},
	}
	h, err := StaticHandler(fsys)
	if err != nil {
		t.Fatalf("StaticHandler: %v", err)
	}

	get := func(path, acceptEncoding string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if acceptEncoding != "" {
			req.Header.Set("Accept-Encoding", acceptEncoding)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	tests := []struct {
		name           string
		path           string
		acceptEncoding string
		wantBody       string
		wantEncoding   string
		wantType       string
		wantCache      string
	}{
		{"immutable asset is cached forever", "/_app/immutable/entry/app.js", "", "console.log(1)", "", "text/javascript; charset=utf-8", "public, max-age=31536000, immutable"},
		{"gzip variant when accepted", "/_app/immutable/entry/app.js", "gzip, deflate, br", "gz-app", "gzip", "text/javascript; charset=utf-8", "public, max-age=31536000, immutable"},
		{"brotli preferred over gzip", "/", "gzip, br", "br-index", "br", "text/html; charset=utf-8", "no-cache"},
		{"q=0 refuses an encoding", "/", "br;q=0, gzip", "gz-index", "gzip", "text/html; charset=utf-8", "no-cache"},
		{"spa fallback is revalidated", "/settings/general", "", "<html>index</html>", "", "text/html; charset=utf-8", "no-cache"},
		{"directory falls back to index", "/_app/immutable/", "", "<html>index</html>", "", "text/html; charset=utf-8", "no-cache"},
		{"plain file without variants", "/favicon.png", "gzip, br", "png", "", "image/png", "no-cache"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := get(tc.path, tc.acceptEncoding)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Errorf("body = %q, want %q", got, tc.wantBody)
			}
			if got := rec.Header().Get("Content-Encoding"); got != tc.wantEncoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tc.wantEncoding)
			}
			if got := rec.Header().Get("Content-Type"); got != tc.wantType {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantType)
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tc.wantCache)
			}
			if got := rec.Header().Get("Vary"); got != "Accept-Encoding" {
				t.Errorf("Vary = %q, want Accept-Encoding", got)
			}
		})
	}
}
