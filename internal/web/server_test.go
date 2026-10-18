package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func newTestServer(ui fstest.MapFS) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	opt := Options{Log: log, Version: "test"}
	if ui != nil {
		opt.UI = ui
	}
	return New(opt).Handler()
}

func get(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

var testUI = fstest.MapFS{
	"index.html":           {Data: []byte("<!doctype html><title>garage</title>")},
	"assets/app-1a2b.js":   {Data: []byte("console.log(1)")},
	"favicon.svg":          {Data: []byte("<svg/>")},
	"manifest.webmanifest": {Data: []byte(`{"name":"garage"}`)},
}

func TestHealth(t *testing.T) {
	rec := get(t, newTestServer(nil), http.MethodGet, "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d", rec.Code)
	}
	var body healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Version != "test" {
		t.Fatalf("ответ %+v", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("ответ API не должен кэшироваться")
	}
}

func TestHealthOnlyGet(t *testing.T) {
	if rec := get(t, newTestServer(nil), http.MethodPost, "/api/health"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/health: код %d, ожидался 405", rec.Code)
	}
}

func TestUnknownAPI(t *testing.T) {
	rec := get(t, newTestServer(testUI), http.MethodGet, "/api/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("код %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatal("неизвестный метод API должен отвечать JSON, а не index.html")
	}
}

func TestSecurityHeaders(t *testing.T) {
	for _, target := range []string{"/api/health", "/", "/assets/app-1a2b.js"} {
		rec := get(t, newTestServer(testUI), http.MethodGet, target)
		h := rec.Header()
		if h.Get("Content-Security-Policy") != contentSecurityPolicy {
			t.Errorf("%s: нет политики безопасности", target)
		}
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "same-origin" {
			t.Errorf("%s: нет служебных заголовков", target)
		}
	}
	if !strings.Contains(contentSecurityPolicy, "frame-ancestors 'none'") || strings.Contains(contentSecurityPolicy, "unsafe") {
		t.Fatal("политика должна запрещать фреймы и не разрешать unsafe-*")
	}
}

func TestSPA(t *testing.T) {
	h := newTestServer(testUI)

	for _, target := range []string{"/", "/items/42", "/places"} {
		rec := get(t, h, http.MethodGet, target)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>garage</title>") {
			t.Errorf("%s: ожидался index.html, код %d", target, rec.Code)
		}
	}

	rec := get(t, h, http.MethodGet, "/assets/app-1a2b.js")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("файл сборки: код %d, Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}

	if rec := get(t, h, http.MethodGet, "/assets/missing.js"); rec.Code != http.StatusNotFound {
		t.Fatalf("отсутствующий файл сборки: код %d, ожидался 404", rec.Code)
	}
	// iOS запрашивает эти файлы до apple-touch-icon.png; HTML с кодом 200
	// вместо картинки даёт на главном экране букву вместо иконки.
	for _, target := range []string{"/apple-touch-icon-precomposed.png", "/favicon.ico"} {
		if rec := get(t, h, http.MethodGet, target); rec.Code != http.StatusNotFound {
			t.Errorf("%s: код %d, ожидался 404", target, rec.Code)
		}
	}

	if rec := get(t, h, http.MethodPost, "/"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /: код %d, ожидался 405", rec.Code)
	}
}

func TestUIMissing(t *testing.T) {
	rec := get(t, newTestServer(nil), http.MethodGet, "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "make web") {
		t.Fatalf("без интерфейса ожидалась подсказка, код %d", rec.Code)
	}
}

func TestManifestType(t *testing.T) {
	rec := get(t, newTestServer(testUI), http.MethodGet, "/manifest.webmanifest")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("манифест: код %d, Content-Type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}
