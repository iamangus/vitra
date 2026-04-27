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
	WikiLinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	tagRegex      = regexp.MustCompile(`(?:^|\s)(#[\w\-/]+)`)
)

func renderMarkdown(content []byte, vaultPath string, index *VaultIndex) (string, error) {
	processed := preprocessObsidianSyntax(content, vaultPath, index)

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

func preprocessObsidianSyntax(content []byte, vaultPath string, index *VaultIndex) []byte {
	text := string(content)

	var protected [][]string

	fencedRegex := regexp.MustCompile("(?s)```.*?```")
	text = fencedRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("\x00PROTECTED_%d\x00", len(protected))
		protected = append(protected, []string{placeholder, match})
		return placeholder
	})

	inlineCodeRegex := regexp.MustCompile("`[^`]+`")
	text = inlineCodeRegex.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := fmt.Sprintf("\x00PROTECTED_%d\x00", len(protected))
		protected = append(protected, []string{placeholder, match})
		return placeholder
	})

	text = WikiLinkRegex.ReplaceAllStringFunc(text, func(match string) string {
		groups := WikiLinkRegex.FindStringSubmatch(match)
		if groups == nil {
			return match
		}

		target := strings.TrimSpace(groups[1])
		display := target
		if len(groups) > 2 && groups[2] != "" {
			display = strings.TrimSpace(groups[2])
		}

		targetPath := findNotePath(target, vaultPath, index)
		if targetPath != "" {
			return fmt.Sprintf(`<a href="/note/%s" class="wikilink">%s</a>`, targetPath, display)
		}
		return fmt.Sprintf(`<a href="/note/%s" class="wikilink missing" hx-confirm="Create note '%s'?">%s</a>`, targetPath, target, display)
	})

	text = tagRegex.ReplaceAllStringFunc(text, func(match string) string {
		groups := tagRegex.FindStringSubmatch(match)
		if groups == nil {
			return match
		}
		tag := groups[1]
		return fmt.Sprintf(` <a href="/search?q=%s" class="tag">%s</a>`, strings.TrimPrefix(tag, "#"), tag)
	})

	for i := len(protected) - 1; i >= 0; i-- {
		text = strings.Replace(text, protected[i][0], protected[i][1], 1)
	}

	return []byte(text)
}

func findNotePath(title string, vaultPath string, index *VaultIndex) string {
	title = strings.TrimSuffix(title, ".md")

	if index != nil {
		if p := index.FindPath(title); p != "" {
			return p
		}
	}

	if strings.Contains(title, "/") || strings.Contains(title, string(filepath.Separator)) {
		candidate := filepath.Join(vaultPath, title+".md")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(vaultPath, candidate)
			return filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
		}
		parts := strings.Split(filepath.ToSlash(title), "/")
		if found := findCaseInsensitivePath(vaultPath, parts); found != "" {
			return found
		}
	}

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
		if i == len(parts)-1 {
			info, err := os.Stat(currentPath + ".md")
			if err == nil && !info.IsDir() {
				rel, _ := filepath.Rel(vaultPath, currentPath+".md")
				return filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
			}
			info, err = os.Stat(currentPath)
			if err == nil && !info.IsDir() {
				rel, _ := filepath.Rel(vaultPath, currentPath)
				return filepath.ToSlash(strings.TrimSuffix(rel, ".md"))
			}
			return ""
		}
		if info, err := os.Stat(currentPath); err != nil || !info.IsDir() {
			return ""
		}
	}
	return ""
}

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
