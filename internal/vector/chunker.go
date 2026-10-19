package vector

import (
	"fmt"
	"strings"
	"unicode"
)

const defaultChunkSize = 1000
const defaultChunkOverlap = 200

func ChunkNote(path, content string, chunkSize, chunkOverlap int, title string, summary ...string) []Chunk {
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	if chunkSize > 4000 {
		chunkSize = 4000
	}
	if chunkOverlap == 0 {
		chunkOverlap = defaultChunkOverlap
	}
	if chunkOverlap < 0 {
		chunkOverlap = 0
	}
	if chunkOverlap >= chunkSize {
		chunkOverlap = chunkSize / 4
	}
	if title == "" {
		title = getTitleFromPath(path)
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if strings.HasPrefix(content, "---\n") {
		if i := strings.Index(content[4:], "\n---\n"); i >= 0 {
			content = content[4+i+5:]
		}
	}
	if strings.TrimSpace(content) == "" && len(summary) > 0 {
		content = summary[0]
	}
	var chunks []Chunk
	var text strings.Builder
	var headings []string
	var headingLevels []int
	fenced := false
	flush := func() {
		runes := []rune(strings.TrimSpace(text.String()))
		text.Reset()
		heading := strings.Join(headings, " > ")
		prefix := fmt.Sprintf("Note: %s | File: %s | Section: %s | ", title, path, heading)
		if len(summary) > 0 && summary[0] != "" {
			prefix += "Summary: " + summary[0] + " | "
		}
		if len(prefix) > 4000 {
			prefix = string([]rune(prefix)[:min(1000, len([]rune(prefix)))])
		}
		for start := 0; start < len(runes); {
			end := min(start+chunkSize, len(runes))
			if end < len(runes) {
				for i := end; i > start+chunkSize/2; i-- {
					if unicode.IsSpace(runes[i-1]) {
						end = i
						break
					}
				}
			}
			chunks = append(chunks, Chunk{Path: path, Title: title, Index: len(chunks), Heading: heading, Text: prefix + string(runes[start:end])})
			if end == len(runes) {
				break
			}
			start = max(start+1, end-chunkOverlap)
		}
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
		}
		level := 0
		if !fenced {
			for level < len(line) && line[level] == '#' {
				level++
			}
		}
		if level > 0 && level <= 6 && len(line) > level && line[level] == ' ' {
			flush()
			for len(headingLevels) > 0 && headingLevels[len(headingLevels)-1] >= level {
				headingLevels = headingLevels[:len(headingLevels)-1]
				headings = headings[:len(headings)-1]
			}
			headings = append(headings, strings.TrimSpace(line[level:]))
			headingLevels = append(headingLevels, level)
		}
		text.WriteString(line)
		text.WriteByte('\n')
	}
	flush()
	return chunks
}

func ChunkNoteSimple(path, content string, chunkSize int) []Chunk {
	return ChunkNote(path, content, chunkSize, -1, "")
}
