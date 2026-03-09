package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaFileSystem wraps an http.FileSystem to serve index.html for unknown routes (SPA fallback).
type spaFileSystem struct {
	fs http.FileSystem
}

func (s spaFileSystem) Open(name string) (http.File, error) {
	f, err := s.fs.Open(name)
	if err != nil && strings.Contains(err.Error(), "no such file") {
		return s.fs.Open("index.html")
	}
	return f, err
}

// StaticHandler returns an http.Handler that serves embedded static files with SPA fallback.
func StaticHandler(staticFS fs.FS) (http.Handler, error) {
	sub, err := fs.Sub(staticFS, "dist")
	if err != nil {
		return nil, err
	}
	return http.FileServer(spaFileSystem{http.FS(sub)}), nil
}
