package server

import (
	"bytes"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	headerVary            = "Vary"
	headerAcceptEncoding  = "Accept-Encoding"
	headerContentEncoding = "Content-Encoding"
	headerCacheControl    = "Cache-Control"

	encodingGzip   = "gzip"
	encodingBrotli = "br"

	spaIndex = "index.html"

	// immutableDir holds the content-hashed build output of SvelteKit: a file
	// there never changes under the same name, so browsers may keep it forever.
	immutableDir       = "_app/immutable/"
	cacheImmutable     = "public, max-age=31536000, immutable"
	cacheRevalidate    = "no-cache"
	mimeOctetStream    = "application/octet-stream"
	qualityParamPrefix = "q="
)

// precompressed lists the encodings the build ships next to each asset, best first.
var precompressed = []struct{ token, ext string }{
	{encodingBrotli, ".br"},
	{encodingGzip, ".gz"},
}

// staticHandler serves the embedded SPA. Unknown paths get index.html (SPA
// fallback); a .br or .gz sibling produced at build time is sent instead of
// the original when the client accepts it, so the Pi never compresses on the fly.
type staticHandler struct {
	fsys fs.FS
}

// StaticHandler returns an http.Handler that serves embedded static files with SPA fallback.
func StaticHandler(staticFS fs.FS) (http.Handler, error) {
	sub, err := fs.Sub(staticFS, "dist")
	if err != nil {
		return nil, err
	}
	return staticHandler{fsys: sub}, nil
}

func (h staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if !h.isFile(name) {
		name = spaIndex
	}

	cache := cacheRevalidate
	if strings.HasPrefix(name, immutableDir) {
		cache = cacheImmutable
	}
	w.Header().Set(headerCacheControl, cache)
	w.Header().Add(headerVary, headerAcceptEncoding)

	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		// Sniffing would look at compressed bytes, so never leave it to ServeContent.
		ctype = mimeOctetStream
	}
	w.Header().Set(headerContentType, ctype)

	file := name
	for _, enc := range precompressed {
		if acceptsEncoding(r, enc.token) && h.isFile(name+enc.ext) {
			file = name + enc.ext
			w.Header().Set(headerContentEncoding, enc.token)
			break
		}
	}

	content, err := h.open(file)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// The embedded files carry no modification time; the cache headers above
	// and the content hash in immutable names do the caching instead.
	http.ServeContent(w, r, name, time.Time{}, content)
}

func (h staticHandler) isFile(name string) bool {
	if name == "" || name == "." {
		return false
	}
	info, err := fs.Stat(h.fsys, name)
	return err == nil && !info.IsDir()
}

// open returns the file as an io.ReadSeeker, as http.ServeContent needs one.
func (h staticHandler) open(name string) (io.ReadSeeker, error) {
	f, err := h.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	if rs, ok := f.(io.ReadSeeker); ok {
		return rs, nil
	}
	defer f.Close() //nolint:errcheck
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// acceptsEncoding reports whether the request's Accept-Encoding lists token
// with a non-zero quality.
func acceptsEncoding(r *http.Request, token string) bool {
	for _, header := range r.Header.Values(headerAcceptEncoding) {
		for _, part := range strings.Split(header, ",") {
			name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
			if !strings.EqualFold(strings.TrimSpace(name), token) {
				continue
			}
			return quality(params) > 0
		}
	}
	return false
}

// quality parses the q parameter of an Accept-Encoding entry; it defaults to 1.
func quality(params string) float64 {
	for _, p := range strings.Split(params, ";") {
		v, ok := strings.CutPrefix(strings.TrimSpace(p), qualityParamPrefix)
		if !ok {
			continue
		}
		q, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0
		}
		return q
	}
	return 1
}
