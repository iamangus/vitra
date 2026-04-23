package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const vaultChangeDebounce = 400 * time.Millisecond

type VaultEvent struct {
	Type    string   `json:"type"`
	Version int64    `json:"version"`
	Paths   []string `json:"paths,omitempty"`
	Tree    bool     `json:"tree,omitempty"`
	Graph   bool     `json:"graph,omitempty"`
	Search  bool     `json:"search,omitempty"`
	Notes   bool     `json:"notes,omitempty"`
}

type LiveSync struct {
	mu           sync.Mutex
	subscribers  map[chan string]struct{}
	pending      VaultEvent
	pendingPaths map[string]struct{}
	debounce     *time.Timer
	version      int64
	watcher      *fsnotify.Watcher
}

func NewLiveSync() *LiveSync {
	return &LiveSync{
		subscribers:  make(map[chan string]struct{}),
		pending:      VaultEvent{Type: "vault.changed"},
		pendingPaths: make(map[string]struct{}),
	}
}

func (ls *LiveSync) Start(vaultPath string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := ls.addRecursiveWatches(vaultPath, vaultPath, watcher); err != nil {
		watcher.Close()
		return err
	}

	ls.mu.Lock()
	ls.watcher = watcher
	ls.mu.Unlock()

	go ls.watch(vaultPath, watcher)
	return nil
}

func (ls *LiveSync) Close() error {
	ls.mu.Lock()
	watcher := ls.watcher
	ls.watcher = nil
	if ls.debounce != nil {
		ls.debounce.Stop()
		ls.debounce = nil
	}
	ls.mu.Unlock()

	if watcher != nil {
		return watcher.Close()
	}
	return nil
}

func (ls *LiveSync) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsubscribe := ls.subscribe()
	defer unsubscribe()

	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case payload := <-ch:
			_, _ = fmt.Fprintf(w, "event: vault\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (ls *LiveSync) Notify(paths []string, tree bool, graph bool, search bool, notes bool) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.pending.Type = "vault.changed"
	ls.pending.Tree = ls.pending.Tree || tree
	ls.pending.Graph = ls.pending.Graph || graph
	ls.pending.Search = ls.pending.Search || search
	ls.pending.Notes = ls.pending.Notes || notes

	for _, path := range paths {
		if path == "" {
			continue
		}
		ls.pendingPaths[path] = struct{}{}
	}

	if ls.debounce != nil {
		ls.debounce.Stop()
	}
	ls.debounce = time.AfterFunc(vaultChangeDebounce, ls.flush)
}

func (ls *LiveSync) flush() {
	ls.mu.Lock()
	if !ls.pending.Tree && !ls.pending.Graph && !ls.pending.Search && !ls.pending.Notes && len(ls.pendingPaths) == 0 {
		ls.debounce = nil
		ls.mu.Unlock()
		return
	}

	event := ls.pending
	ls.version += 1
	event.Version = ls.version
	event.Paths = make([]string, 0, len(ls.pendingPaths))
	for path := range ls.pendingPaths {
		event.Paths = append(event.Paths, path)
	}
	sort.Strings(event.Paths)

	subscribers := make([]chan string, 0, len(ls.subscribers))
	for ch := range ls.subscribers {
		subscribers = append(subscribers, ch)
	}

	ls.pending = VaultEvent{Type: "vault.changed"}
	ls.pendingPaths = make(map[string]struct{})
	ls.debounce = nil
	ls.mu.Unlock()

	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	for _, ch := range subscribers {
		select {
		case ch <- string(payload):
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- string(payload):
			default:
			}
		}
	}
}

func (ls *LiveSync) subscribe() (chan string, func()) {
	ch := make(chan string, 1)
	ls.mu.Lock()
	ls.subscribers[ch] = struct{}{}
	ls.mu.Unlock()

	return ch, func() {
		ls.mu.Lock()
		delete(ls.subscribers, ch)
		ls.mu.Unlock()
	}
}

func (ls *LiveSync) watch(vaultPath string, watcher *fsnotify.Watcher) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			ls.handleWatcherEvent(vaultPath, watcher, event)
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (ls *LiveSync) handleWatcherEvent(vaultPath string, watcher *fsnotify.Watcher, event fsnotify.Event) {
	if shouldIgnoreVaultPath(vaultPath, event.Name) {
		return
	}

	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			_ = ls.addRecursiveWatches(vaultPath, event.Name, watcher)
			if relPath, _, ok := normalizeVaultPath(vaultPath, event.Name, false); ok {
				ls.Notify([]string{relPath}, true, false, false, false)
			}
			return
		}
	}

	normalizedPath, isMarkdown, ok := normalizeVaultPath(vaultPath, event.Name, true)
	if !ok {
		return
	}

	treeChanged := !isMarkdown && event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
	if isMarkdown && event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
		treeChanged = true
	}

	if !treeChanged && !isMarkdown {
		return
	}

	ls.Notify(
		[]string{normalizedPath},
		treeChanged,
		isMarkdown,
		isMarkdown,
		isMarkdown,
	)
}

func (ls *LiveSync) addRecursiveWatches(vaultPath string, root string, watcher *fsnotify.Watcher) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if shouldIgnoreVaultPath(vaultPath, path) {
			if path == root {
				return nil
			}
			return filepath.SkipDir
		}
		return watcher.Add(path)
	})
}

func normalizeVaultPath(vaultPath string, path string, trimMarkdownExt bool) (string, bool, bool) {
	relPath, err := filepath.Rel(vaultPath, path)
	if err != nil {
		return "", false, false
	}
	relPath = filepath.ToSlash(relPath)
	if relPath == "." || relPath == "" {
		return "", false, false
	}

	isMarkdown := strings.EqualFold(filepath.Ext(relPath), ".md")
	if trimMarkdownExt && isMarkdown {
		relPath = strings.TrimSuffix(relPath, filepath.Ext(relPath))
	}
	return relPath, isMarkdown, true
}

func shouldIgnoreVaultPath(vaultPath string, path string) bool {
	relPath, err := filepath.Rel(vaultPath, path)
	if err != nil {
		return true
	}
	relPath = filepath.ToSlash(relPath)
	if relPath == "." || relPath == "" {
		return false
	}

	for _, part := range strings.Split(relPath, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func (fs *FileSystem) HandleAPIEvents(w http.ResponseWriter, r *http.Request) {
	if fs.live == nil {
		http.Error(w, "Live updates unavailable", http.StatusServiceUnavailable)
		return
	}
	fs.live.HandleSSE(w, r)
}
