package internal

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SkillMeta is the metadata exposed for each skill in the vault's skills
// directory via GET /api/skills. Path is the vault-relative path used to
// address the skill with the note tools (e.g. read_note, write_note).
type SkillMeta struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Path        string   `json:"path"`
	Type        string   `json:"type,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Size        int64    `json:"size"`
	Mtime       string   `json:"mtime"`
}

// ListSkillMetadata scans the vault's skills directory and returns metadata
// for every skill markdown file, sorted by name. Path values are
// vault-relative so they can be used directly with the note tools. Missing or
// unreadable directories yield an empty list.
func (fs *FileSystem) ListSkillMetadata() ([]SkillMeta, error) {
	dir := filepath.Join(fs.VaultPath, fs.skillsDirRel)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []SkillMeta{}, nil
	}
	var skills []SkillMeta
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		fullPath := filepath.Join(dir, name)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fm, body := ParseNote(content)
		base := strings.TrimSuffix(name, ".md")
		meta := SkillMeta{
			Name:  base,
			Path:  fs.skillsDirRel + "/" + base,
			Size:  info.Size(),
			Mtime: info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
		}
		if fm != nil {
			if s, ok := fm["title"].(string); ok && s != "" {
				meta.Title = s
			}
			if s, ok := fm["description"].(string); ok && s != "" {
				meta.Description = s
			}
			if s, ok := fm["type"].(string); ok && s != "" {
				meta.Type = s
			}
			if tags, ok := fm["tags"]; ok {
				meta.Tags = parseTags(tags)
			}
		}
		if meta.Title == "" {
			meta.Title = meta.Name
		}
		if meta.Description == "" {
			meta.Description = firstParagraph(string(body))
		}
		skills = append(skills, meta)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// HandleAPISkills serves GET /api/skills: a JSON array of skill metadata.
// Intended for deterministic, programmatic inclusion in system prompts.
func (fs *FileSystem) HandleAPISkills(w http.ResponseWriter, r *http.Request) {
	skills, err := fs.ListSkillMetadata()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, skills)
}

func parseTags(v interface{}) []string {
	switch t := v.(type) {
	case string:
		t = strings.Trim(t, "[]")
		var out []string
		for _, p := range strings.Split(t, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	case []interface{}:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	}
	return nil
}

func firstParagraph(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}
