package internal

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSearchSnippet(t *testing.T) {
	for _, tc := range []struct {
		name, content, query, contains string
	}{
		{"body", "---\ntitle: Secret metadata\n---\n\nFind this idea", "idea", "Find this idea"},
		{"unicode", strings.Repeat("한글 café ", 60) + "UNIQUE connection" + strings.Repeat(" after", 100), "unique", "UNIQUE connection"},
		{"metadata match", "---\ntitle: metadata\n---\n\nReadable body", "metadata", "Readable body"},
		{"empty", "", "none", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := searchSnippet(tc.content, tc.query)
			if !utf8.ValidString(got) || len([]rune(got)) > 242 || !strings.Contains(got, tc.contains) || strings.Contains(got, "Secret metadata") {
				t.Fatalf("unexpected snippet: %q", got)
			}
		})
	}
}
