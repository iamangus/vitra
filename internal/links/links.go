package links

import (
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var wiki = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

func Resolve(target, base string) string {
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil || u.IsAbs() || u.Host != "" || u.Path == "" {
		return ""
	}
	decoded := u.Path
	if strings.Contains(decoded, "\\") {
		return ""
	}
	if strings.HasPrefix(decoded, "/") {
		decoded = strings.TrimPrefix(decoded, "/")
	} else {
		decoded = path.Join(base, decoded)
	}
	decoded = path.Clean(decoded)
	if decoded == ".." || strings.HasPrefix(decoded, "../") || decoded == "." {
		return ""
	}
	for _, part := range strings.Split(decoded, "/") {
		if strings.HasPrefix(part, ".") {
			return ""
		}
	}
	return strings.TrimSuffix(decoded, ".md")
}

func Extract(body []byte, base string, includeWiki bool) []string {
	doc := goldmark.DefaultParser().Parse(text.NewReader(body))
	seen := map[string]bool{}
	var prose strings.Builder
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if node.Type() == ast.TypeBlock {
				prose.WriteByte('\n')
			}
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.CodeBlock, *ast.FencedCodeBlock, *ast.CodeSpan, *ast.RawHTML, *ast.HTMLBlock:
			prose.WriteByte('\n')
			return ast.WalkSkipChildren, nil
		case *ast.Link:
			u, err := url.Parse(string(n.Destination))
			if err == nil && strings.HasSuffix(u.Path, ".md") {
				if target := Resolve(string(n.Destination), base); target != "" {
					seen[target] = true
				}
			}
			prose.WriteByte('\n')
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			prose.Write(n.Segment.Value(body))
			if n.SoftLineBreak() || n.HardLineBreak() {
				prose.WriteByte('\n')
			}
		case *ast.String:
			prose.Write(n.Value)
		}
		return ast.WalkContinue, nil
	})
	if includeWiki {
		for _, m := range wiki.FindAllStringSubmatch(prose.String(), -1) {
			target := strings.TrimSpace(strings.SplitN(m[1], "#", 2)[0])
			target = strings.TrimSuffix(target, ".md")
			if target != "" {
				seen[target] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for target := range seen {
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}
