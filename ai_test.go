package main

import (
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeOpenCode answers like OpenCode: it describes images by the colour
// of their first pixel and embeds texts as bags of words, so texts that
// share words are close.
type fakeOpenCode struct {
	mu       sync.Mutex
	chats    int
	embeds   int
	headers  http.Header
	status   int // answer chats with this when not 0
	retry    string
	resetsAt string // when Go's usage says the rate-limited window resets
	lastBody map[string]any
	paths    []string
}

func (f *fakeOpenCode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.headers = r.Header.Clone()
	f.paths = append(f.paths, r.URL.Path)
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.lastBody = body
	if r.Header.Get("Authorization") != "Bearer good-key" {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key","type":"AuthError"}}`))
		return
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/usage"):
		_, _ = w.Write([]byte(`{"usage":{"rolling":{"status":"rate-limited","percent":100,"resetsAt":"` + f.resetsAt + `"},"weekly":{"status":"ok","percent":40,"resetsAt":"2099-01-01T00:00:00Z"}}}`))
	case strings.HasSuffix(r.URL.Path, "/chat/completions"):
		f.chats++
		if f.status != 0 {
			if f.retry != "" {
				w.Header().Set("Retry-After", f.retry)
			}
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
			return
		}
		url := body["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["image_url"].(map[string]any)["url"].(string)
		data, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/jpeg;base64,"))
		img, _, err := image.Decode(strings.NewReader(string(data)))
		text := "A plain grey square.\nKeywords: grey, square"
		if err == nil {
			r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
			switch {
			case r > g && r > b:
				text = "<think>red things</think>**A red apple** on a wooden table.\nKeywords: apple, fruit, red"
			case b > r && b > g:
				text = "A blue ocean under a clear sky.\nKeywords: sea, ocean, water, blue"
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": text}}}})
	case strings.HasSuffix(r.URL.Path, "/embeddings"):
		f.embeds++
		var data []any
		for i, t := range body["input"].([]any) {
			data = append(data, map[string]any{"index": i, "embedding": bagOfWords(t.(string))})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	default:
		http.NotFound(w, r)
	}
}

var (
	vocabMu sync.Mutex
	vocab   = map[string]int{}
)

// bagOfWords counts each word of s in a dimension of its own.
func bagOfWords(s string) []float64 {
	vocabMu.Lock()
	defer vocabMu.Unlock()
	v := make([]float64, embedDims)
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r < 'a' || r > 'z' }) {
		i, ok := vocab[w]
		if !ok {
			i = len(vocab) % embedDims
			vocab[w] = i
		}
		v[i]++
	}
	return v
}

func writePNG(t *testing.T, path string, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// testLibrary makes a library of a folder with a red and a blue image,
// whose AI talks to fake.
func testLibrary(t *testing.T, fake *fakeOpenCode) (*Library, *AI) {
	t.Helper()
	dir := t.TempDir()
	l := newLibrary()
	l.dataDir = filepath.Join(dir, "data")
	l.cacheDir = filepath.Join(dir, "cache")
	_ = os.MkdirAll(l.dataDir, 0o755)
	_ = os.MkdirAll(l.cacheDir, 0o755)
	pics := filepath.Join(dir, "pics")
	_ = os.MkdirAll(pics, 0o755)
	f := &folderState{Folder: Folder{ID: "f1", Name: "pics", Path: pics}}
	for i, c := range []color.Color{color.RGBA{220, 30, 30, 255}, color.RGBA{30, 30, 220, 255}} {
		name := []string{"apple.png", "sea.png"}[i]
		p := filepath.Join(pics, name)
		writePNG(t, p, c)
		img := &Image{ID: shortHash(p), Folder: "f1", Name: name, Path: p, Kind: KindPNG, Version: shortHash(p + "v1")}
		l.images[img.ID] = img
		f.ids = append(f.ids, img.ID)
	}
	l.folders = []*folderState{f}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	a := l.ai
	a.zenURL, a.goURL, a.goUsageURL = srv.URL+"/zen/v1", srv.URL+"/go/v1", srv.URL+"/go/usage"
	a.key, a.keyRead = "good-key", true
	return l, a
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDescribeAndSearch(t *testing.T) {
	fake := &fakeOpenCode{}
	l, a := testLibrary(t, fake)
	a.sync()
	waitFor(t, "descriptions", func() bool { return a.status().Described == 2 })
	waitFor(t, "vectors", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return len(a.vecs) == 2 && len(a.embedWait) == 0
	})

	var apple *Image
	for _, img := range l.images {
		if img.Name == "apple.png" {
			apple = img
		}
	}
	if want := "A red apple on a wooden table.\nKeywords: apple, fruit, red"; apple.Description != want {
		t.Errorf("description = %q, want %q", apple.Description, want)
	}
	if got := fake.headers.Get("x-opencode-session"); !strings.HasPrefix(got, "eagle-") {
		t.Errorf("session header = %q", got)
	}
	if got := fake.headers.Get("User-Agent"); !strings.HasPrefix(got, "Eagle/") {
		t.Errorf("user agent = %q", got)
	}

	hits, err := a.Search(t.Context(), "red fruit")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != apple.ID {
		t.Errorf("search for red fruit = %+v, want only the apple, %s", hits, apple.ID)
	}

	// Descriptions and vectors are saved and read back.
	a.saveDescriptions()
	a.saveVectors()
	waitFor(t, "saved files", func() bool { return fileExists(a.descPath()) && fileExists(a.vecPath()) })
	b := newAI(l)
	b.load()
	if got := b.descriptionOf(apple.Version); got != apple.Description {
		t.Errorf("loaded description = %q", got)
	}
	if len(b.vecs) != 2 {
		t.Errorf("loaded %d vectors, want 2", len(b.vecs))
	}

	// Nothing is described twice.
	chats := fake.chats
	a.sync()
	time.Sleep(200 * time.Millisecond)
	if fake.chats != chats {
		t.Errorf("sync described again: %d chats, then %d", chats, fake.chats)
	}
	// Unless asked.
	if err := a.Describe([]string{apple.ID}, true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "describing again", func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return fake.chats == chats+1
	})
}

func TestDescribesOnGoAndEmbedsOnZen(t *testing.T) {
	fake := &fakeOpenCode{}
	_, a := testLibrary(t, fake)
	if _, err := a.Test(t.Context(), "", "gpt-6-luna"); err != nil {
		t.Fatal(err)
	}
	if fake.lastBody["model"] != "gpt-6-luna" {
		t.Errorf("model = %v", fake.lastBody["model"])
	}
	if _, err := a.Test(t.Context(), "bad-key", ""); err == nil || !strings.Contains(err.Error(), "didn't accept the API key") {
		t.Errorf("bad key: %v", err)
	}
	a.sync()
	waitFor(t, "descriptions and vectors", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return len(a.vecs) == 2
	})
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, p := range fake.paths {
		if strings.HasSuffix(p, "/chat/completions") && p != "/go/v1/chat/completions" {
			t.Errorf("described at %s, not on Go", p)
		}
		if strings.HasSuffix(p, "/embeddings") && p != "/zen/v1/embeddings" {
			t.Errorf("embedded at %s, not on Zen", p)
		}
	}
}

func TestRateLimitPauses(t *testing.T) {
	fake := &fakeOpenCode{status: 429, retry: "3600"}
	_, a := testLibrary(t, fake)
	a.sync()
	waitFor(t, "pause", func() bool { return a.status().Paused != "" })
	st := a.status()
	if !strings.Contains(st.Paused, "Go allowance") {
		t.Errorf("paused = %q", st.Paused)
	}
	// The image that hit the limit waits with the rest.
	if st.Pending != 2 {
		t.Errorf("pending = %d, want 2", st.Pending)
	}
	fake.mu.Lock()
	fake.status = 0
	fake.mu.Unlock()
	a.Resume()
	waitFor(t, "descriptions after resuming", func() bool { return a.status().Described == 2 })
}

func TestRateLimitWaitsForGoReset(t *testing.T) {
	reset := time.Now().Add(3 * time.Hour).Truncate(time.Second)
	fake := &fakeOpenCode{status: 429, resetsAt: reset.UTC().Format(time.RFC3339)}
	_, a := testLibrary(t, fake)
	a.sync()
	waitFor(t, "pause", func() bool { return a.status().Paused != "" })
	st := a.status()
	if want := reset.Add(30 * time.Second).Format("15:04"); !strings.Contains(st.Paused, want) || !strings.Contains(st.Paused, "Go allowance") {
		t.Errorf("paused = %q, want it to name Go and %s", st.Paused, want)
	}
	a.mu.Lock()
	timer := a.resume != nil
	a.mu.Unlock()
	if !timer {
		t.Error("no timer resumes describing")
	}
	if got := time.UnixMilli(st.ResumesAt); got.Sub(reset) < 0 || got.Sub(reset) > time.Minute {
		t.Errorf("resumes at %v, want just after %v", got, reset)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, p := range fake.paths {
		if strings.HasPrefix(p, "/zen/") && strings.HasSuffix(p, "/chat/completions") {
			t.Errorf("described on Zen while Go was limited: %s", p)
		}
	}
}

func TestParseTime(t *testing.T) {
	want := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	for _, raw := range []string{`"2026-11-23T08:00:00Z"`, `1795420800`, `1795420800000`, `"1795420800"`} {
		got, ok := parseTime(json.RawMessage(raw))
		if !ok || !got.Equal(want) {
			t.Errorf("parseTime(%s) = %v, %v", raw, got, ok)
		}
	}
	if _, ok := parseTime(json.RawMessage(`null`)); ok {
		t.Error("parseTime(null) is a time")
	}
}

func TestBadKeyPauses(t *testing.T) {
	fake := &fakeOpenCode{}
	_, a := testLibrary(t, fake)
	a.key = "bad-key"
	a.sync()
	waitFor(t, "pause", func() bool { return a.status().Paused != "" })
	if st := a.status(); !strings.Contains(st.Paused, "API key") || st.Described != 0 {
		t.Errorf("status = %+v", st)
	}
}

func TestEmbeddingRefusedTurnsSearchOff(t *testing.T) {
	fake := &fakeOpenCode{}
	_, a := testLibrary(t, fake)
	a.mu.Lock()
	a.searchErr = ""
	a.mu.Unlock()
	a.searchFailed(&apiError{status: 402, message: "Insufficient balance"})
	st := a.status()
	if st.Search || !strings.Contains(st.SearchError, "balance") {
		t.Errorf("status = %+v", st)
	}
	if hits, err := a.Search(t.Context(), "apple"); hits != nil || err != nil {
		t.Errorf("search while off = %v, %v", hits, err)
	}
}

func TestFriendlyErrors(t *testing.T) {
	cases := []struct {
		err  *apiError
		want string
	}{
		{&apiError{status: 403, message: "Model access is disabled. Change model access at https://opencode.ai/console/x/go."}, "turned off for your OpenCode workspace"},
		{&apiError{status: 403, message: "EntitlementError: no Go subscription"}, "no OpenCode Go subscription"},
		{&apiError{status: 403, message: "Something else"}, "Something else"},
		{&apiError{status: 401}, "didn't accept the API key"},
		{&apiError{status: 402, message: "Upstream request failed: Endpoint is unavailable."}, "Endpoint is unavailable"},
		{&apiError{status: 402, message: "Insufficient balance"}, "balance has run out"},
	}
	for _, c := range cases {
		if got := friendly(c.err, goService).Error(); !strings.Contains(got, c.want) {
			t.Errorf("friendly(%v) = %q, want it to say %q", c.err, got, c.want)
		}
	}
}

func TestCleanDescription(t *testing.T) {
	cases := map[string]string{
		"<think>hmm</think>\n\n## Description: A cat.\n\n**Keywords:** cat, pet": "A cat.\nKeywords: cat, pet",
		"  A dog.  ":                             "A dog.",
		"A dog on a beach. Keywords: dog, beach": "A dog on a beach.\nKeywords: dog, beach",
		"一只狗。关键词：狗，海滩":                           "一只狗。\n关键词: 狗，海滩",
	}
	for in, want := range cases {
		if got := cleanDescription(in); got != want {
			t.Errorf("cleanDescription(%q) = %q, want %q", in, got, want)
		}
	}
	if got := contentText(json.RawMessage(`[{"type":"text","text":"A "},{"type":"text","text":"cat."}]`)); got != "A cat." {
		t.Errorf("contentText of parts = %q", got)
	}
}

func TestRank(t *testing.T) {
	ids := func(hits []SearchHit) string {
		var out []string
		for _, h := range rank(hits) {
			out = append(out, h.ID)
		}
		return strings.Join(out, " ")
	}
	cases := []struct {
		name string
		hits []SearchHit
		want string
	}{
		// Scores from text-embedding-3-large over eight drawings.
		{"one stands out", []SearchHit{{"venice", .42}, {"umbrella", .30}, {"split", .29}, {"garden", .29}, {"poppies", .28}, {"butterfly", .26}, {"skyline", .25}, {"mansion", .23}}, "venice"},
		{"nothing matches", []SearchHit{{"umbrella", .30}, {"butterfly", .28}, {"venice", .24}, {"garden", .24}, {"skyline", .24}, {"split", .23}, {"mansion", .21}, {"poppies", .20}}, ""},
		{"a few match", []SearchHit{{"umbrella", .35}, {"skyline", .32}, {"butterfly", .27}, {"split", .27}, {"venice", .24}, {"poppies", .19}, {"garden", .18}, {"mansion", .16}}, "umbrella skyline"},
		{"most match", []SearchHit{{"poppies", .48}, {"split", .47}, {"skyline", .46}, {"umbrella", .45}, {"garden", .38}, {"butterfly", .38}, {"mansion", .37}, {"venice", .28}}, "poppies split skyline umbrella"},
	}
	for _, c := range cases {
		if got := ids(c.hits); got != c.want {
			t.Errorf("%s: rank kept %q, want %q", c.name, got, c.want)
		}
	}
}
