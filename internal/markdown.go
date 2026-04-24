package internal

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

var (
	wikiLinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	tagRegex      = regexp.MustCompile(`(?:^|\s)(#[\w\-/]+)`)
)

func renderMarkdown(content []byte, vaultPath string) (string, error) {
	// Pre-process wiki links and tags before goldmark
	processed := preprocessObsidianSyntax(content, vaultPath)

	var buf bytes.Buffer
	md := goldmark.New(
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	if err := md.Convert(processed, &buf); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func preprocessObsidianSyntax(content []byte, vaultPath string) []byte {
	text := string(content)

	// Protect code blocks and inline code from WikiLink/tag processing
	var protected [][]string

	// Protect fenced code blocks
	fencedRegex := regexp.MustCompile("(?s)```.*?```")
	text = fencedRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("\x00PROTECTED_%d\x00", len(protected))
		protected = append(protected, []string{placeholder, match})
		return placeholder
	})

	// Protect inline code
	inlineCodeRegex := regexp.MustCompile("`[^`]+`")
	text = inlineCodeRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("\x00PROTECTED_%d\x00", len(protected))
		protected = append(protected, []string{placeholder, match})
		return placeholder
	})

	// WikiLinks: [[Note Name]] or [[Note Name|Display Text]]
	text = wikiLinkRegex.ReplaceAllStringFunc(text, func(match string) string {
		groups := wikiLinkRegex.FindStringSubmatch(match)
		if groups == nil {
			return match
		}

		target := strings.TrimSpace(groups[1])
		display := target
		if len(groups) > 2 && groups[2] != "" {
			display = strings.TrimSpace(groups[2])
		}

		// Check if target exists
		targetPath := findNotePath(target, vaultPath)
		if targetPath != "" {
			return fmt.Sprintf(`<a href="/note/%s" class="wikilink">%s</a>`, targetPath, display)
		}
		// Link to create page
		return fmt.Sprintf(`<a href="/note/%s" class="wikilink missing" hx-confirm="Create note '%s'?">%s</a>`, targetPath, target, display)
	})

	// Tags: #tag
	text = tagRegex.ReplaceAllStringFunc(text, func(match string) string {
		groups := tagRegex.FindStringSubmatch(match)
		if groups == nil {
			return match
		}
		tag := groups[1]
		return fmt.Sprintf(` <a href="/search?q=%s" class="tag">%s</a>`, strings.TrimPrefix(tag, "#"), tag)
	})

	// Restore protected content
	for i := len(protected) - 1; i >= 0; i-- {
		text = strings.Replace(text, protected[i][0], protected[i][1], 1)
	}

	return []byte(text)
}

func findNotePath(title string, vaultPath string) string {
	// Strip .md extension if present
	title = strings.TrimSuffix(title, ".md")

	// If title contains a path separator, try exact path match first
	if strings.Contains(title, "/") || strings.Contains(title, string(filepath.Separator)) {
		candidate := filepath.Join(vaultPath, title+".md")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(vaultPath, candidate)
			return filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
		}
		// Try case-insensitive path match by walking parent directories
		parts := strings.Split(filepath.ToSlash(title), "/")
		if found := findCaseInsensitivePath(vaultPath, parts); found != "" {
			return found
		}
	}

	// Fallback: match by filename only across the entire vault
	var found string
	filepath.Walk(vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.EqualFold(strings.TrimSuffix(info.Name(), ".md"), filepath.Base(title)) {
			rel, _ := filepath.Rel(vaultPath, path)
			found = filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// findCaseInsensitivePath walks the vault trying to match each path segment case-insensitively.
func findCaseInsensitivePath(vaultPath string, parts []string) string {
	currentPath := vaultPath
	for i, part := range parts {
		entries, err := os.ReadDir(currentPath)
		if err != nil {
			return ""
		}
		var matched bool
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				currentPath = filepath.Join(currentPath, entry.Name())
				matched = true
				break
			}
		}
		if !matched {
			return ""
		}
		// Last part must be a file (with .md extension)
		if i == len(parts)-1 {
			info, err := os.Stat(currentPath + ".md")
			if err == nil && !info.IsDir() {
				rel, _ := filepath.Rel(vaultPath, currentPath+".md")
				return filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
			}
			// Also check if it's already a file without adding .md
			info, err = os.Stat(currentPath)
			if err == nil && !info.IsDir() {
				rel, _ := filepath.Rel(vaultPath, currentPath)
				return filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
			}
			return ""
		}
		// Intermediate parts must be directories
		if info, err := os.Stat(currentPath); err != nil || !info.IsDir() {
			return ""
		}
	}
	return ""
}

// Extract frontmatter and body from markdown content
func parseNote(content []byte) (frontmatter map[string]interface{}, body []byte) {
	frontmatter = make(map[string]interface{})
	body = content

	if !bytes.HasPrefix(content, []byte("---\n")) {
		return
	}

	parts := bytes.SplitN(content, []byte("---\n"), 3)
	if len(parts) < 3 {
		return
	}

	// Simple YAML parsing for common fields
	fmText := string(parts[1])
	lines := strings.Split(fmText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			value = strings.Trim(value, `"'`)
			frontmatter[key] = value
		}
	}

	body = bytes.TrimSpace(parts[2])
	return
}

func buildBreadcrumbs(path string) []map[string]string {
	var crumbs []map[string]string
	parts := strings.Split(path, "/")
	for i := 0; i < len(parts)-1; i++ {
		crumbs = append(crumbs, map[string]string{
			"Name": parts[i],
			"Path": strings.Join(parts[:i+1], "/"),
		})
	}
	return crumbs
}
