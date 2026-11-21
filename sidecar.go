package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// A sidecar is a text file saved next to an image that says what the image
// is: the metadata file gallery-dl writes with --write-metadata
// (photo.jpg.json), or a plain note (photo.txt). Eagle shows its text as
// the image's caption.

const (
	maxSidecarBytes = 1 << 20 // larger files are not metadata
	maxCaptionRunes = 5000
)

// isSidecar tells whether a file name could be a sidecar.
func isSidecar(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json", ".txt":
		return true
	}
	return false
}

// sidecarFor returns the sidecar of the image at path, among the sidecar
// files found next to it ("" if it has none). photo.jpg.json is gallery-dl's
// name; photo.json, photo.jpg.txt and photo.txt are also looked for.
func sidecarFor(path string, sidecars map[string]bool) string {
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	for _, p := range []string{path + ".json", stem + ".json", path + ".txt", stem + ".txt"} {
		if sidecars[p] {
			return p
		}
	}
	return ""
}

// Sidecar is what a sidecar says about its image.
type Sidecar struct {
	Caption string
	Author  string
	Link    string
}

// readSidecar reads the sidecar at path. A file it can't read or make sense
// of gives an empty Sidecar.
func readSidecar(path string) Sidecar {
	f, err := os.Open(path)
	if err != nil {
		return Sidecar{}
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxSidecarBytes+1))
	if err != nil || len(b) > maxSidecarBytes || !utf8.Valid(b) {
		return Sidecar{}
	}
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return parseMetadata(b)
	}
	return Sidecar{Caption: cleanText(string(b))}
}

// parseMetadata reads a gallery-dl metadata file. For X (Twitter) it gives
// the tweet's text, its author and a link to it; for other sites, the text
// and author fields most of gallery-dl's extractors use.
func parseMetadata(b []byte) Sidecar {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber() // tweet IDs don't fit in a float64
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return Sidecar{}
	}
	var s Sidecar
	for _, k := range []string{"content", "description", "caption", "title", "text"} {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			s.Caption = cleanText(v)
			break
		}
	}

	category, _ := m["category"].(string)
	twitter := category == "twitter" || category == "x"
	handle, display := "", ""
	if a, ok := m["author"].(map[string]any); ok {
		handle, _ = a["name"].(string)
		display, _ = a["nick"].(string)
	}
	if handle == "" {
		for _, k := range []string{"username", "uploader", "user"} {
			if v, ok := m[k].(string); ok && v != "" {
				handle = v
				break
			}
		}
	}
	switch {
	case twitter && handle != "" && display != "" && display != handle:
		s.Author = fmt.Sprintf("%s (@%s)", display, handle)
	case twitter && handle != "":
		s.Author = "@" + handle
	case display != "":
		s.Author = display
	default:
		s.Author = handle
	}

	if twitter && handle != "" {
		if id := idString(m["tweet_id"]); id != "" {
			s.Link = "https://x.com/" + handle + "/status/" + id
		}
	}
	if s.Link == "" {
		for _, k := range []string{"post_url", "webpage_url"} {
			if v, ok := m[k].(string); ok && isWebLink(v) {
				s.Link = v
				break
			}
		}
	}
	return s
}

var digits = regexp.MustCompile(`^[0-9]+$`)

func idString(v any) string {
	var s string
	switch v := v.(type) {
	case json.Number:
		s = v.String()
	case string:
		s = v
	}
	if !digits.MatchString(s) {
		return ""
	}
	return s
}

func isWebLink(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

var (
	lineBreakTag = regexp.MustCompile(`(?i)<br\s*/?>|</p>`)
	anyTag       = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	blankLines   = regexp.MustCompile(`\n{3,}`)
)

// cleanText turns a caption into plain text: the HTML some sites use
// becomes text, line endings are made uniform, and very long text is cut.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if strings.Contains(s, "<") {
		s = lineBreakTag.ReplaceAllString(s, "\n")
		s = anyTag.ReplaceAllString(s, "")
	}
	s = html.UnescapeString(s)
	s = blankLines.ReplaceAllString(strings.TrimSpace(s), "\n\n")
	if utf8.RuneCountInString(s) > maxCaptionRunes {
		r := []rune(s)
		s = strings.TrimSpace(string(r[:maxCaptionRunes])) + "…"
	}
	return s
}
