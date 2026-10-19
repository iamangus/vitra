package internal

import (
	"encoding/json"
	"testing"
	"time"
)

func TestActivityAndSlowSubscriberInvalidation(t *testing.T) {
	live := NewLiveSync()
	defer live.Close()
	ch, unsubscribe := live.subscribe()
	defer unsubscribe()
	live.Notify([]string{"first"}, true, true, true, true)
	live.flush()
	live.Notify([]string{"log"}, false, false, false, true)
	live.flush()
	select {
	case payload := <-ch:
		var event VaultEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			t.Fatal(err)
		}
		if !event.Activity || !event.Tree || !event.Graph || !event.Notes {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing event")
	}
}

func TestGraphBacklinksAndClosureShareLinkParsing(t *testing.T) {
	f := testFS(t)
	for path, content := range map[string]string{"folder/one": "[two](../two.md) [[three]] `[[ignored]]`", "two": "body", "three": "body"} {
		if err := f.CreateNote(path, content); err != nil {
			t.Fatal(err)
		}
	}
	_, edges := f.index.GetGraph()
	if len(edges) != 2 {
		t.Fatal(edges)
	}
	backlinks, err := f.NoteBacklinks("two")
	if err != nil || len(backlinks) != 1 {
		t.Fatal(backlinks, err)
	}
	closure, err := f.TransitiveClosure("folder/one", 2)
	if err != nil || len(closure) != 2 {
		t.Fatal(closure, err)
	}
	html, err := renderMarkdown([]byte("[two](../two.md)"), f.VaultPath, f.index, "folder")
	if err != nil {
		t.Fatal(err)
	}
	if html != "<p><a href=\"/note/two\" rel=\"nofollow\">two</a></p>\n" {
		t.Fatal(html)
	}
}
