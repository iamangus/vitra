package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityMiddleware(t *testing.T) {
	handler := SecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), []string{"wiki.example"})
	for _, test := range []struct {
		host, origin, site string
		status             int
	}{
		{"127.0.0.1:8080", "", "", 204}, {"wiki.example", "https://wiki.example", "same-origin", 204}, {"evil.example", "", "", 403}, {"localhost:8080", "https://evil.example", "", 403}, {"localhost", "null", "", 403}, {"localhost", "", "cross-site", 403},
	} {
		t.Run(test.host+test.origin+test.site, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/notes", nil)
			r.Host = test.host
			r.Header.Set("Origin", test.origin)
			r.Header.Set("Sec-Fetch-Site", test.site)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatal(w.Code)
			}
			if w.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("missing CSP")
			}
		})
	}
}

func TestHTTPRevisionsAndNotFound(t *testing.T) {
	f := testFS(t)
	r := httptest.NewRequest("GET", "/api/note/missing", nil)
	r.SetPathValue("path", "missing")
	w := httptest.NewRecorder()
	f.HandleAPIViewNote(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err := f.CreateNote("one", "original"); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("POST", "/api/note/one", nil)
	r.SetPathValue("path", "one")
	w = httptest.NewRecorder()
	f.HandleAPISaveNote(w, r)
	if w.Code != 428 {
		t.Fatal(w.Code)
	}
}
