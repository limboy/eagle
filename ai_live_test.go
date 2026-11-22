package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLiveOpenCode talks to OpenCode itself. It runs only with an API key
// in OPENCODE_API_KEY. It describes the images of EAGLE_LIVE_DIR (up to
// EAGLE_LIVE_MAX, 8 by default) on Go with EAGLE_LIVE_MODEL,
// or takes their descriptions from the JSON file EAGLE_LIVE_DESC (file
// name to description), then prints how close each is to the queries of
// EAGLE_LIVE_QUERIES, separated by "|", for each embedding model of
// EAGLE_LIVE_EMBED, separated by ",":
//
//	OPENCODE_API_KEY=… EAGLE_LIVE_DIR=~/Pictures/X go test -run TestLiveOpenCode -v
func TestLiveOpenCode(t *testing.T) {
	key := os.Getenv("OPENCODE_API_KEY")
	dir := os.Getenv("EAGLE_LIVE_DIR")
	if key == "" || dir == "" {
		t.Skip("set OPENCODE_API_KEY and EAGLE_LIVE_DIR to talk to OpenCode")
	}
	limit := 8
	if n, err := strconv.Atoi(os.Getenv("EAGLE_LIVE_MAX")); err == nil && n > 0 {
		limit = n
	}
	s := normalize(AISettings{Model: os.Getenv("EAGLE_LIVE_MODEL"), Language: os.Getenv("EAGLE_LIVE_LANGUAGE"), Search: true})
	queries := strings.Split(os.Getenv("EAGLE_LIVE_QUERIES"), "|")
	if os.Getenv("EAGLE_LIVE_QUERIES") == "" {
		queries = []string{"a person", "nature outdoors", "food", "an animal", "city at night"}
	}
	models := strings.Split(os.Getenv("EAGLE_LIVE_EMBED"), ",")
	if os.Getenv("EAGLE_LIVE_EMBED") == "" {
		models = []string{embedModel}
	}
	given := map[string]string{}
	if p := os.Getenv("EAGLE_LIVE_DESC"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &given); err != nil {
			t.Fatal(err)
		}
	}

	tmp := t.TempDir()
	l := newLibrary()
	l.dataDir, l.cacheDir = filepath.Join(tmp, "data"), filepath.Join(tmp, "cache")
	_ = os.MkdirAll(l.dataDir, 0o755)
	_ = os.MkdirAll(l.cacheDir, 0o755)
	a := l.ai
	a.key, a.keyRead = key, true
	a.settings = s

	var imgs []*Image
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || len(imgs) >= limit {
			return nil
		}
		kind, ok := kindByExt[strings.ToLower(filepath.Ext(p))]
		if !ok {
			return nil
		}
		img := &Image{ID: shortHash(p), Name: filepath.Base(p), Path: p, Kind: kind, Version: shortHash(p + "live")}
		imgs = append(imgs, img)
		l.images[img.ID] = img
		return nil
	})
	if len(imgs) == 0 {
		t.Fatalf("no images in %s", dir)
	}

	for _, img := range imgs {
		if d, ok := given[img.Name]; ok {
			img.Description = cleanDescription(d)
			continue
		}
		start := time.Now()
		if err := a.describeOne(key, s, img.ID, false); err != nil {
			t.Fatalf("%s: %v (%v)", img.Name, err, friendly(err, goService))
		}
		t.Logf("%s, %s, %v:\n%s\n", img.Name, s.Model, time.Since(start).Round(time.Millisecond), img.Description)
	}

	texts := l.searchTexts()
	var ids, inputs []string
	for id, text := range texts {
		ids = append(ids, id)
		inputs = append(inputs, text)
	}
	defer func(m string) { embedModel = m }(embedModel)
	for _, m := range models {
		embedModel = strings.TrimSpace(m)
		start := time.Now()
		vs, err := a.embed(t.Context(), key, inputs, false)
		if err != nil {
			t.Errorf("%s: %v (%v)", embedModel, err, friendly(err, zenService))
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "== %s, %d texts in %v\n", embedModel, len(inputs), time.Since(start).Round(time.Millisecond))
		for _, q := range queries {
			q = strings.TrimSpace(q)
			qv, err := a.embed(t.Context(), key, []string{q}, true)
			if err != nil {
				t.Fatal(err)
			}
			var all []SearchHit
			for i, id := range ids {
				all = append(all, SearchHit{ID: id, Score: dot(qv[0], vs[i])})
			}
			sort.Slice(all, func(i, j int) bool { return all[i].Score > all[j].Score })
			kept := map[string]bool{}
			for _, h := range rank(append([]SearchHit(nil), all...)) {
				kept[h.ID] = true
			}
			fmt.Fprintf(&b, "%-26s", q)
			for _, h := range all {
				mark := " "
				if kept[h.ID] {
					mark = "*"
				}
				fmt.Fprintf(&b, " %s%.2f %s", mark, h.Score, label(l.images[h.ID]))
			}
			b.WriteString("\n")
		}
		t.Log("\n" + b.String())
	}
}

// label names an image by its file name, or by EAGLE_LIVE_LABELS, a JSON
// file of file name to label.
func label(img *Image) string {
	labelsOnce.Do(func() {
		if p := os.Getenv("EAGLE_LIVE_LABELS"); p != "" {
			if b, err := os.ReadFile(p); err == nil {
				_ = json.Unmarshal(b, &labels)
			}
		}
	})
	if l, ok := labels[img.Name]; ok {
		return l
	}
	return img.Name
}

var (
	labelsOnce sync.Once
	labels     map[string]string
)
