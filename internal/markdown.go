package internal

import (
	"bytes"
	"fmt"
	"github.com/iamangus/vitra/internal/links"
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	stdhtml "html"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
)

var (
	WikiLinkRegex = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	tagRegex      = regexp.MustCompile(`(?:^|\s)(#[\w\-/]+)`)
)

func renderMarkdown(content []byte, vaultPath string, index *VaultIndex, base ...string) (string, error) {
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

	nodes, err := nethtml.ParseFragment(strings.NewReader(buf.String()), &nethtml.Node{Type: nethtml.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return "", err
	}
	dir := ""
	if len(base) > 0 {
		dir = base[0]
	}
	var rewrite func(*nethtml.Node)
	rewrite = func(node *nethtml.Node) {
		if node.Type == nethtml.ElementNode && node.Data == "a" {
			for i, attr := range node.Attr {
				if attr.Key == "href" {
					u, err := url.Parse(attr.Val)
					if err == nil && strings.HasSuffix(u.Path, ".md") {
						if target := links.Resolve(attr.Val, dir); target != "" {
							node.Attr[i].Val = "/note/" + url.PathEscape(target)
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			rewrite(child)
		}
	}
	buf.Reset()
	for _, node := range nodes {
		rewrite(node)
		if err := nethtml.Render(&buf, node); err != nil {
			return "", err
		}
	}
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class").Matching(regexp.MustCompile(`^(wikilink( missing)?|tag)$`)).OnElements("a")
	return policy.Sanitize(buf.String()), nil
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
			return fmt.Sprintf(`<a href="/note/%s" class="wikilink">%s</a>`, url.PathEscape(targetPath), stdhtml.EscapeString(display))
		}
		return fmt.Sprintf(`<a href="/note/%s" class="wikilink missing">%s</a>`, url.PathEscape(target), stdhtml.EscapeString(display))
	})

	text = tagRegex.ReplaceAllStringFunc(text, func(match string) string {
		groups := tagRegex.FindStringSubmatch(match)
		if groups == nil {
			return match
		}
		tag := groups[1]
		return fmt.Sprintf(` <a href="/search?q=%s" class="tag">%s</a>`, url.QueryEscape(strings.TrimPrefix(tag, "#")), stdhtml.EscapeString(tag))
	})

	for i := len(protected) - 1; i >= 0; i-- {
		text = strings.Replace(text, protected[i][0], protected[i][1], 1)
	}

	return []byte(text)
}

func findNotePath(title string, vaultPath string, index *VaultIndex) string {
	title = strings.TrimSuffix(title, ".md")

	if index != nil {
		return index.FindPath(title)
	}

	if strings.Contains(title, "/") || strings.Contains(title, string(filepath.Separator)) {
		candidate, err := safeVaultPath(vaultPath, title+".md")
		if err != nil {
			return ""
		}
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

func ParseNote(content []byte) (frontmatter map[string]interface{}, body []byte) {
	return parseNote(content)
}

func parseNote(content []byte) (frontmatter map[string]interface{}, body []byte) {
	frontmatter = make(map[string]interface{})
	body = content
	fm, rest, err := splitFrontmatter(content)
	if err != nil || fm == nil {
		return
	}
	if err := yaml.Unmarshal(fm, &frontmatter); err != nil {
		return map[string]interface{}{}, content
	}
	for key, value := range frontmatter {
		frontmatter[key] = jsonMetadata(value)
	}
	body = rest
	return
}

func jsonMetadata(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[interface{}]interface{}:
		out := map[string]interface{}{}
		for key, item := range typed {
			out[fmt.Sprint(key)] = jsonMetadata(item)
		}
		return out
	case map[string]interface{}:
		for key, item := range typed {
			typed[key] = jsonMetadata(item)
		}
		return typed
	case []interface{}:
		for i, item := range typed {
			typed[i] = jsonMetadata(item)
		}
		return typed
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return fmt.Sprint(typed)
		}
	}
	return value
}

func splitFrontmatter(content []byte) ([]byte, []byte, error) {
	lines := bytes.SplitAfter(content, []byte("\n"))
	if len(lines) == 0 || strings.TrimRight(string(lines[0]), "\r\n") != "---" {
		return nil, content, nil
	}
	offset := len(lines[0])
	start := offset
	for _, line := range lines[1:] {
		if strings.TrimRight(string(line), "\r\n") == "---" {
			return content[start:offset], content[offset+len(line):], nil
		}
		offset += len(line)
	}
	return nil, content, fmt.Errorf("unterminated frontmatter")
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
