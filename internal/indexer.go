package internal

import (
	"context"
	"errors"
	"time"

	"github.com/iamangus/vitra/internal/okf"
	"github.com/iamangus/vitra/internal/vector"
)

func (fs *FileSystem) noteChunks(path string, data []byte) []vector.Chunk {
	fm, body := parseNote(data)
	meta := okf.Extract(fm)
	chunks := vector.ChunkNote(path, string(body), fs.chunkSize, fs.chunkOverlap, meta.Title, meta.Summary)
	for i := range chunks {
		chunks[i].Type = meta.Type
		chunks[i].Tags = meta.Tags
	}
	return chunks
}

func (fs *FileSystem) queueIndex() {
	if fs.indexQueue == nil {
		return
	}
	select {
	case fs.indexQueue <- struct{}{}:
	default:
	}
}

func (fs *FileSystem) startIndexer() {
	ctx, cancel := context.WithCancel(context.Background())
	fs.indexCancel = cancel
	fs.indexQueue = make(chan struct{}, 1)
	fs.indexDone = make(chan struct{})
	fs.indexedHashes = map[string]string{}
	if fs.live != nil {
		fs.live.SetReconcile(fs.queueIndex)
	}
	go func() {
		defer close(fs.indexDone)
		retry := time.NewTicker(30 * time.Second)
		defer retry.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-fs.indexQueue:
			case <-retry.C:
			}
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if err := fs.ReconcileIndex(ctx); err != nil && !errors.Is(err, context.Canceled) {
				fs.recordIndexError(err)
			}
		}
	}()
	fs.queueIndex()
}

func (fs *FileSystem) ReconcileIndex(ctx context.Context) error {
	fs.indexRunMu.Lock()
	defer fs.indexRunMu.Unlock()
	if fs.index != nil {
		if err := fs.index.Build(fs.VaultPath); err != nil {
			return err
		}
	}
	files := map[string][]byte{}
	fs.mutationMu.Lock()
	err := walkNotes(fs.VaultPath, "", fs.skillsDirRel, true, func(path string, data []byte) error { files[path] = data; return ctx.Err() })
	fs.mutationMu.Unlock()
	if err != nil {
		return err
	}
	if len(fs.indexedHashes) == 0 {
		if replacer, ok := fs.VectorStore.(interface {
			ReplaceNotes(context.Context, map[string][]vector.Chunk) error
		}); ok {
			notes := make(map[string][]vector.Chunk, len(files))
			for p, data := range files {
				notes[p] = fs.noteChunks(p, data)
			}
			if err := replacer.ReplaceNotes(ctx, notes); err != nil {
				return err
			}
			for p, data := range files {
				fs.indexedHashes[p] = revision(data)
			}
			fs.statusMu.Lock()
			fs.indexError = ""
			fs.statusMu.Unlock()
			return nil
		}
	}
	var failures []error
	for path, data := range files {
		if fs.indexedHashes[path] == revision(data) {
			continue
		}
		if err := fs.VectorStore.IndexNote(ctx, path, fs.noteChunks(path, data)); err != nil {
			failures = append(failures, err)
		} else {
			fs.indexedHashes[path] = revision(data)
		}
	}
	paths := make([]string, 0, len(fs.indexedHashes))
	for path := range fs.indexedHashes {
		paths = append(paths, path)
	}
	if store, ok := fs.VectorStore.(interface{ Paths() []string }); ok {
		paths = append(paths, store.Paths()...)
	}
	for _, path := range paths {
		if _, exists := files[path]; exists {
			continue
		}
		if err := fs.VectorStore.DeleteNote(ctx, path); err != nil {
			failures = append(failures, err)
		} else {
			delete(fs.indexedHashes, path)
		}
	}
	err = errors.Join(failures...)
	if err == nil {
		fs.statusMu.Lock()
		fs.indexError = ""
		fs.statusMu.Unlock()
	}
	return err
}
