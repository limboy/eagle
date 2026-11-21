package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMetadataTwitter(t *testing.T) {
	// The shape of gallery-dl's metadata for a tweet, trimmed.
	b := []byte(`{
		"category": "twitter", "subcategory": "tweet",
		"tweet_id": 1867384012345678901,
		"author": {"id": 12, "name": "nasa", "nick": "NASA"},
		"content": "Saturn&#39;s rings, up close &amp; in color.\r\nMore soon.",
		"filename": "G1ekEipW4AA1I3y", "extension": "jpg", "num": 1
	}`)
	s := parseMetadata(b)
	if want := "Saturn's rings, up close & in color.\nMore soon."; s.Caption != want {
		t.Errorf("caption = %q, want %q", s.Caption, want)
	}
	if s.Author != "NASA (@nasa)" {
		t.Errorf("author = %q", s.Author)
	}
	// The ID is beyond float64's precision: it must come through exactly.
	if want := "https://x.com/nasa/status/1867384012345678901"; s.Link != want {
		t.Errorf("link = %q, want %q", s.Link, want)
	}
}

func TestParseMetadataOtherSites(t *testing.T) {
	s := parseMetadata([]byte(`{"category": "pixiv", "title": "Study", "caption": "<p>Morning light<br/>on the hill</p>",
		"user": {"name": "ignored"}, "post_url": "https://example.com/p/1"}`))
	if s.Caption != "Morning light\non the hill" {
		t.Errorf("caption = %q", s.Caption)
	}
	if s.Link != "https://example.com/p/1" {
		t.Errorf("link = %q", s.Link)
	}
	if s := parseMetadata([]byte(`{"post_url": "javascript:alert(1)", "title": "x"}`)); s.Link != "" {
		t.Errorf("non-web link kept: %q", s.Link)
	}
	if s := parseMetadata([]byte(`not json`)); s != (Sidecar{}) {
		t.Errorf("bad JSON gave %+v", s)
	}
}

func TestSidecarFor(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "photo.jpg")
	note := filepath.Join(dir, "photo.txt")
	meta := filepath.Join(dir, "photo.jpg.json")
	set := map[string]bool{note: true}
	if got := sidecarFor(img, set); got != note {
		t.Errorf("sidecarFor = %q, want the note", got)
	}
	set[meta] = true
	if got := sidecarFor(img, set); got != meta {
		t.Errorf("sidecarFor = %q, want gallery-dl's file first", got)
	}
	if got := sidecarFor(filepath.Join(dir, "other.jpg"), set); got != "" {
		t.Errorf("sidecarFor matched another image's file: %q", got)
	}

	if err := os.WriteFile(note, []byte("  A note\r\n\r\n\r\n\r\nwith a gap  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := readSidecar(note); s.Caption != "A note\n\nwith a gap" {
		t.Errorf("txt caption = %q", s.Caption)
	}
}
