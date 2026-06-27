package mcp

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// StartSkillsServer exposes each markdown skill file under skillsDir as an MCP
// tool. The tool takes no parameters and returns the full skill file content.
// Filesystem changes under skillsDir are watched and the tool set is reconciled
// live via SetTools. Blocks until the server stops.
func StartSkillsServer(skillsDir string, port string) error {
	if info, err := os.Stat(skillsDir); err != nil || !info.IsDir() {
		return fmt.Errorf("skills directory not found: %s", skillsDir)
	}

	srv := server.NewMCPServer(
		"vitra-skills",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	lm := &skillsLoader{dir: skillsDir, srv: srv}
	lm.reload()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("skills: fsnotify unavailable: %v (live skill updates disabled)", err)
	} else {
		defer watcher.Close()
		if err := watcher.Add(skillsDir); err != nil {
			log.Printf("skills: watch %s: %v", skillsDir, err)
		}
		go func() {
			for {
				select {
				case ev, ok := <-watcher.Events:
					if !ok {
						return
					}
					if !strings.HasSuffix(ev.Name, ".md") {
						continue
					}
					log.Printf("skills: change detected (%s), reconciling", ev.Op)
					lm.reload()
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					log.Printf("skills: watcher error: %v", err)
				}
			}
		}()
	}

	return server.NewStreamableHTTPServer(srv).Start(":" + port)
}

// skillsLoader scans skillsDir for *.md files, parses each file's frontmatter
// to derive a tool name and description, and (re)registers them all on the MCP
// server via SetTools (which replaces the previous set atomically).
type skillsLoader struct {
	dir string
	srv *server.MCPServer

	mu        sync.Mutex
	registered map[string]bool
}

func (lm *skillsLoader) reload() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	files, err := filepath.Glob(filepath.Join(lm.dir, "*.md"))
	if err != nil {
		log.Printf("skills: glob %s: %v", lm.dir, err)
		return
	}

	tools := make([]server.ServerTool, 0, len(files))
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			log.Printf("skills: read %s: %v", f, err)
			continue
		}
		name, desc := parseSkillMetadata(filepath.Base(f), content)
		if !isValidToolName(name) {
			log.Printf("skills: skipping %s: invalid tool name %q", f, name)
			continue
		}
		filePath := f
		tool := mcp.NewTool(name, mcp.WithDescription(desc))
		handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			data, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("read skill file: %w", err)
			}
			return mcp.NewToolResultText(string(data)), nil
		}
		tools = append(tools, server.ServerTool{Tool: tool, Handler: handler})
		log.Printf("skills: registered tool %q from %s", name, filepath.Base(f))
	}

	lm.srv.SetTools(tools...)
	lm.registered = make(map[string]bool, len(tools))
	for _, t := range tools {
		lm.registered[t.Tool.Name] = true
	}
}

// parseSkillMetadata extracts a tool name and description from a skill markdown
// file. It reads YAML-ish frontmatter (delimited by ---) for `name` and
// `description` keys. Falls back to a sanitized filename (name) and the first
// non-empty heading or paragraph (description) when frontmatter is absent.
func parseSkillMetadata(filename string, content []byte) (name, description string) {
	name = sanitizeToolName(strings.TrimSuffix(filename, ".md"))
	description = fmt.Sprintf("Skill: %s", name)

	text := string(content)
	if strings.HasPrefix(text, "---") {
		end := strings.Index(text[3:], "\n---")
		if end >= 0 {
			fm := text[3 : 3+end]
			for _, line := range strings.Split(fm, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				idx := strings.Index(line, ":")
				if idx < 0 {
					continue
				}
				key := strings.TrimSpace(line[:idx])
				val := strings.TrimSpace(strings.Trim(line[idx+1:], `"`))
				switch strings.ToLower(key) {
				case "name":
					if val != "" {
						name = sanitizeToolName(val)
					}
				case "description":
					if val != "" {
						description = val
					}
				}
			}
		}
	}

	if description == "" || strings.HasPrefix(description, "Skill:") {
		if d := firstNonEmptyDescription(text); d != "" {
			description = d
		}
	}
	return name, description
}

func firstNonEmptyDescription(text string) string {
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "---") {
			continue
		}
		if strings.HasPrefix(t, "#") {
			return strings.TrimSpace(strings.TrimLeft(t, "# "))
		}
		return t
	}
	return ""
}

var toolNameRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func isValidToolName(name string) bool {
	return name != "" && toolNameRe.MatchString(name)
}

func sanitizeToolName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == ' ':
			b.WriteRune('_')
		default:
			if i == 0 {
				b.WriteRune('s')
			}
		}
	}
	out := b.String()
	if out == "" {
		return "skill"
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "s_" + out
	}
	return out
}