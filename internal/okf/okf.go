// Package okf implements the Open Knowledge Format v0.1 conventions.
//
// OKF represents knowledge as a directory of markdown files with YAML
// frontmatter. The only hard requirement is that every concept document
// carries a non-empty `type` field in its frontmatter. This package
// extracts OKF metadata from parsed frontmatter, recognizes OKF cross-link
// forms in markdown bodies, and parses reserved `log.md` files.
//
// Reference: https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
package okf

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// DefaultType is the implicit concept type applied at read time to notes that
// do not declare a `type` field on disk. This keeps vitra's vault spec-conformant
// without rewriting existing notes.
const DefaultType = "Note"

// Metadata is the OKF projection of a note's frontmatter. All fields except
// Type are optional per the spec; Type defaults to DefaultType when absent.
type Metadata struct {
	Type        string   `json:"type"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Summary     string   `json:"summary,omitempty"`
	Resource    string   `json:"resource,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Timestamp   string   `json:"timestamp,omitempty"`
	Version     string   `json:"okf_version,omitempty"`
}

// Extract projects a parsed frontmatter map (as returned by internal.parseNote)
// into an OKF Metadata. Missing `type` is filled with DefaultType. Unknown keys
// are tolerated and ignored — consumers MUST NOT reject documents with
// unrecognized frontmatter (spec §9).
func Extract(frontmatter map[string]interface{}) Metadata {
	m := Metadata{Type: DefaultType}
	if frontmatter == nil {
		return m
	}
	if v, ok := frontmatter["type"]; ok {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			m.Type = strings.TrimSpace(s)
		}
	}
	if v, ok := frontmatter["title"]; ok {
		if s, ok := v.(string); ok {
			m.Title = s
		}
	}
	if v, ok := frontmatter["description"]; ok {
		if s, ok := v.(string); ok {
			m.Description = s
		}
	}
	if v, ok := frontmatter["summary"]; ok {
		if s, ok := v.(string); ok {
			m.Summary = s
		}
	}
	if v, ok := frontmatter["resource"]; ok {
		if s, ok := v.(string); ok {
			m.Resource = s
		}
	}
	if v, ok := frontmatter["tags"]; ok {
		switch t := v.(type) {
		case []interface{}:
			for _, item := range t {
				if s, ok := item.(string); ok {
					m.Tags = append(m.Tags, s)
				}
			}
		case string:
			for _, s := range strings.Split(t, ",") {
				if s = strings.TrimSpace(s); s != "" {
					m.Tags = append(m.Tags, s)
				}
			}
		}
	}
	if v, ok := frontmatter["timestamp"]; ok {
		if s, ok := v.(string); ok {
			m.Timestamp = s
		}
	}
	if v, ok := frontmatter["okf_version"]; ok {
		if s, ok := v.(string); ok {
			m.Version = s
		}
	}
	return m
}

// IsReservedFilename reports whether name is one of the OKF reserved
// filenames (index.md or log.md). Comparison is case-insensitive on the
// basename only.
func IsReservedFilename(name string) bool {
	base := name
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		base = name[i+1:]
	}
	base = strings.ToLower(base)
	return base == "index.md" || base == "log.md"
}

// LogEntry is one parsed entry from a log.md file (spec §7).
type LogEntry struct {
	Date string `json:"date"` // ISO 8601 YYYY-MM-DD
	Kind string `json:"kind"` // e.g. "Update", "Creation", "Deprecation"
	Text string `json:"text"` // entry prose
}

// logDateRegex matches an H2-level date heading `## YYYY-MM-DD`.
var logDateRegex = regexp.MustCompile(`^##\s+(\d{4}-\d{2}-\d{2})\s*$`)

// logEntryRegex matches `* **Kind**: text` bullet lines.
var logEntryRegex = regexp.MustCompile(`^\s*[\*\-]\s+\*\*([^*]+)\*\*\s*:\s*(.+)$`)

// ParseLogEntries parses a log.md body into date-grouped entries, newest
// first. Date headings MUST use ISO 8601 YYYY-MM-DD form (spec §7). Entries
// without a preceding date heading are dropped. Returns an empty slice for
// empty/unparseable input.
func ParseLogEntries(body []byte) []LogEntry {
	var entries []LogEntry
	var currentDate string
	for _, line := range strings.Split(string(body), "\n") {
		if m := logDateRegex.FindStringSubmatch(line); m != nil {
			currentDate = m[1]
			continue
		}
		if currentDate == "" {
			continue
		}
		if m := logEntryRegex.FindStringSubmatch(line); m != nil {
			entries = append(entries, LogEntry{
				Date: currentDate,
				Kind: strings.TrimSpace(m[1]),
				Text: strings.TrimSpace(m[2]),
			})
		}
	}
	// Sort newest first by date, preserving input order within a date.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Date > entries[j].Date
	})
	return entries
}

// absLinkRegex matches absolute bundle-relative links: [text](/foo/bar.md).
var absLinkRegex = regexp.MustCompile(`\[[^\]]*\]\(/([^)]+\.md)\)`)

// relLinkRegex matches relative links: [text](./other.md) or [text](foo/other.md).
// Excludes external URLs (http://, https://, mailto:, #anchors, tel:).
var relLinkRegex = regexp.MustCompile(`\[[^\]]*\]\((\.?/)?((?:[^:\]#!][^\]#!]*\.md))(?:\s+"[^"]*")?\)`)

// ExtractOKFLinks returns the set of OKF cross-link target concept IDs
// (paths with the .md suffix stripped) found in a markdown body. Both
// absolute `/foo/bar.md` and relative `./other.md` / `foo/other.md` forms
// are recognized per spec §5. External URLs and `[[wikilinks]]` (handled
// separately by markdown.go) are ignored. Returned concept IDs are not
// de-duplicated against the vault — broken links are tolerated per §5.3.
//
// The optional basePath (the directory of the linking note, e.g. "metrics")
// is used to resolve relative links to a bundle-relative concept ID. When
// basePath is empty, relative links are returned as-is (still .md-stripped).
func ExtractOKFLinks(body []byte, basePath string) []string {
	seen := make(map[string]struct{})
	var links []string

	add := func(p string) {
		p = strings.TrimSuffix(strings.TrimSpace(p), ".md")
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		links = append(links, p)
	}

	for _, m := range absLinkRegex.FindAllStringSubmatch(string(body), -1) {
		add(m[1])
	}
	for _, m := range relLinkRegex.FindAllStringSubmatch(string(body), -1) {
		p := m[2]
		if basePath != "" && !strings.HasPrefix(p, "/") {
			p = strings.TrimSuffix(basePath, "/") + "/" + strings.TrimPrefix(p, "./")
		}
		add(p)
	}

	sort.Strings(links)
	return links
}

// FormatTimestamp returns an ISO 8601 timestamp suitable for the `timestamp`
// frontmatter field, in UTC.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}