package internal

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func SecurityMiddleware(next http.Handler, allowedHosts []string) http.Handler {
	allowed := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	for _, host := range allowedHosts {
		if host = strings.TrimSpace(strings.ToLower(host)); host != "" {
			allowed[host] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' https: data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		host := r.Host
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		if !allowed[strings.ToLower(host)] {
			http.Error(w, "Host not allowed", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/mcp" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, r.Host) {
					http.Error(w, "Origin not allowed", http.StatusForbidden)
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "Cross-site request rejected", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func (fs *FileSystem) HandleStatus(w http.ResponseWriter, r *http.Request) {
	fs.statusMu.Lock()
	message := fs.indexError
	fs.statusMu.Unlock()
	status := "ready"
	watcherError := ""
	if fs.live != nil {
		watcherError = fs.live.WatchError()
	}
	if message != "" || watcherError != "" {
		status = "degraded"
	}
	writeJSON(w, map[string]interface{}{"status": status, "semantic_error": message, "watcher_error": watcherError, "authentication": "disabled"})
}

func (fs *FileSystem) HandleAPISkills(w http.ResponseWriter, r *http.Request) {
	metadata, err := fs.ListSkillMetadata()
	if err != nil {
		apiError(w, err)
		return
	}
	if metadata == nil {
		metadata = []SkillMeta{}
	}
	writeJSON(w, metadata)
}
