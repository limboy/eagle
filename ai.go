package main

// AI describes images with a vision model and searches them by meaning.
// Both run through OpenCode: descriptions always on Go, the subscription,
// waiting for its allowance when it runs out, and embeddings on Zen, the
// only one of the two that serves them.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// service is the part of OpenCode a request goes to.
type service string

const (
	// goService is OpenCode Go, the subscription, which describes images.
	goService service = "go"
	// zenService is OpenCode Zen, paid from the workspace's balance,
	// which embeds texts.
	zenService service = "zen"
)

// AISettings are what the user chose for describing and searching images.
type AISettings struct {
	// Model is the vision model of OpenCode Go that describes images.
	Model string `json:"model"`
	// Language is what descriptions are written in, such as "English".
	Language string `json:"language"`
	// Auto describes every image of the library, new ones as they come;
	// otherwise only those the user asks for.
	Auto bool `json:"auto"`
	// Search embeds descriptions and captions on Zen, for search by
	// meaning; otherwise search only matches their words.
	Search bool `json:"search"`
}

// AIModel is a vision model the user can pick.
type AIModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Price is what it costs per million tokens in and out, in dollars.
	Price string `json:"price"`
}

// AIConfig is the settings with what the settings page shows beside them.
type AIConfig struct {
	Settings AISettings `json:"settings"`
	// HasKey tells whether an API key is saved.
	HasKey bool `json:"hasKey"`
	// Models are the vision models of OpenCode Go to choose from.
	Models []AIModel `json:"models"`
}

// AIStatus is how far describing and embedding have come.
type AIStatus struct {
	// Ready is true once an API key is saved.
	Ready bool `json:"ready"`
	Auto  bool `json:"auto"`
	// Search is true when search by meaning is on and working.
	Search bool `json:"search"`
	// Described and Total count the library's images with a description,
	// and all of them.
	Described int `json:"described"`
	Total     int `json:"total"`
	// Pending counts the images waiting to be described or being
	// described now.
	Pending int `json:"pending"`
	// Working lists the IDs of the images being described now.
	Working []string `json:"working"`
	// Paused tells why describing stopped, "" while it runs.
	Paused string `json:"paused"`
	// ResumesAt is when describing goes on by itself, in Unix
	// milliseconds, as when Go's allowance resets; 0 when it waits for
	// the user.
	ResumesAt int64 `json:"resumesAt"`
	// SearchError tells why search by meaning is off although the user
	// turned it on, "" when it works.
	SearchError string `json:"searchError"`
	// LastError is the latest failure that didn't stop describing.
	LastError string `json:"lastError"`
}

// Description is an image's new description.
type Description struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// SearchHit is an image that matches a search, with how closely.
type SearchHit struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// AIStatusChanged is sent whenever describing makes progress or stops.
var AIStatusChanged = mygo.NewEvent[AIStatus]("aiStatus")

// DescriptionsChanged is sent in batches as images are described.
var DescriptionsChanged = mygo.NewEvent[[]Description]("descriptions")

var visionModels = []AIModel{
	{ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", Price: "$0.15 / $0.60"},
	{ID: "claude-haiku-5-5", Name: "Claude Haiku 5.5", Price: "$0.10 / $0.50"},
	{ID: "gpt-6-luna", Name: "GPT-6 Luna", Price: "$0.10 / $0.50"},
	{ID: "qwen3.8-flash", Name: "Qwen3.8 Flash", Price: "$0.15 / $0.47"},
	{ID: "glm-5.3-flash", Name: "GLM-5.3 Flash", Price: "$0.15 / $0.50"},
	{ID: "mimo-v2.6-flash", Name: "MiMo-V2.6 Flash", Price: "$0.14 / $0.28"},
	{ID: "minimax-m3.1-flash", Name: "MiniMax M3.1 Flash", Price: "$0.15 / $0.60"},
	{ID: "kimi-k3", Name: "Kimi K3", Price: "$3 / $15"},
}

const (
	defaultModel = "deepseek-v4.1-flash"
	// workers is how many images are described at once.
	workers = 3
	// keyName is the API key's name in the Keychain.
	keyName = "opencode-api-key"
)

// embedModel and embedDims make the vectors search compares: vectors are
// cut to embedDims, which models trained for it (all of Zen's) allow. The
// model is part of each vector's key, so another embeds everything again.
var (
	embedModel = "text-embedding-3-large"
	embedDims  = 512
)

func defaultSettings() AISettings {
	return AISettings{Model: defaultModel, Language: "English", Auto: true, Search: true}
}

type savedDesc struct {
	Text  string `json:"t"`
	Model string `json:"m"`
}

type vectorFile struct {
	Model string
	Dims  int
	Vecs  map[string][]float32
}

// AI is the service pages call to set up and use descriptions and search.
type AI struct {
	lib *Library

	mu       sync.Mutex
	settings AISettings
	key      string
	keyRead  bool
	// desc holds descriptions by image version, so a changed file is
	// described again.
	desc map[string]savedDesc
	// vecs holds unit vectors by embedKey of their text.
	vecs map[string][]float32

	queue   []string        // image IDs waiting to be described
	queued  map[string]bool // what queue holds
	redo    map[string]bool // queued images to describe again
	working map[string]bool
	failed  map[string]bool // images that failed this session
	running int
	paused  string
	resume  *time.Timer
	resumes time.Time
	fails   int // failures in a row, which pause describing

	embedWait  map[string]string // texts waiting for vectors, by key
	embedTimer *time.Timer
	embedBusy  bool
	searchErr  string
	lastErr    string

	queryVecs map[string][]float32

	outMu    sync.Mutex
	out      []Description
	outTimer *time.Timer

	statusTimer *time.Timer
	saveTimer   *time.Timer
	vecTimer    *time.Timer

	session string
	client  *http.Client
	// Base URLs, which tests point elsewhere.
	zenURL, goURL, goUsageURL string
}

func newAI(lib *Library) *AI {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &AI{
		lib:        lib,
		settings:   defaultSettings(),
		desc:       map[string]savedDesc{},
		vecs:       map[string][]float32{},
		queued:     map[string]bool{},
		redo:       map[string]bool{},
		working:    map[string]bool{},
		failed:     map[string]bool{},
		embedWait:  map[string]string{},
		queryVecs:  map[string][]float32{},
		session:    "eagle-" + hex.EncodeToString(b),
		client:     &http.Client{Timeout: 2 * time.Minute},
		zenURL:     "https://opencode.ai/inference/openai/v1",
		goURL:      "https://opencode.ai/inference/go/openai/v1",
		goUsageURL: "https://opencode.ai/inference/go/v1/usage",
	}
}

// load reads the settings, descriptions and vectors saved before. The
// library calls it before listing its folders.
func (a *AI) load() {
	var s AISettings
	if b, err := os.ReadFile(a.settingsPath()); err == nil && json.Unmarshal(b, &s) == nil {
		a.settings = normalize(s)
	}
	desc := map[string]savedDesc{}
	if b, err := os.ReadFile(a.descPath()); err == nil {
		_ = json.Unmarshal(b, &desc)
	}
	var vf vectorFile
	if f, err := os.Open(a.vecPath()); err == nil {
		if gob.NewDecoder(f).Decode(&vf) != nil || vf.Model != embedModel || vf.Dims != embedDims {
			vf.Vecs = nil
		}
		f.Close()
	}
	a.mu.Lock()
	a.desc = desc
	if vf.Vecs != nil {
		a.vecs = vf.Vecs
	}
	a.mu.Unlock()
	// The Keychain may ask the user, so the key is read off the way of
	// startup.
	go func() {
		a.apiKey()
		a.sync()
	}()
}

func normalize(s AISettings) AISettings {
	d := defaultSettings()
	if s.Model == "" {
		s.Model = d.Model
	}
	s.Language = strings.TrimSpace(s.Language)
	if s.Language == "" {
		s.Language = d.Language
	}
	return s
}

func (a *AI) settingsPath() string { return filepath.Join(a.lib.dataDir, "ai.json") }
func (a *AI) descPath() string     { return filepath.Join(a.lib.dataDir, "descriptions.json") }
func (a *AI) vecPath() string      { return filepath.Join(a.lib.cacheDir, "vectors.gob") }

// apiKey returns the saved API key, or OPENCODE_API_KEY from the
// environment without one, and "" without either.
func (a *AI) apiKey() string {
	a.mu.Lock()
	if a.keyRead {
		k := a.key
		a.mu.Unlock()
		return k
	}
	a.mu.Unlock()
	key := ""
	if b, err := mygo.Secrets.Get(keyName); err == nil {
		key = strings.TrimSpace(string(b))
	} else if !errors.Is(err, mygo.ErrSecretNotFound) {
		log.Printf("eagle: reading the API key: %v", err)
	}
	if key == "" {
		key = strings.TrimSpace(os.Getenv("OPENCODE_API_KEY"))
	}
	a.mu.Lock()
	a.key, a.keyRead = key, true
	a.mu.Unlock()
	return key
}

// ---- pages call these ----

// Config returns the settings, whether a key is saved and the models.
func (a *AI) Config() AIConfig {
	key := a.apiKey()
	a.mu.Lock()
	defer a.mu.Unlock()
	return AIConfig{Settings: a.settings, HasKey: key != "", Models: visionModels}
}

// Status returns how far describing has come.
func (a *AI) Status() AIStatus {
	a.apiKey()
	return a.status()
}

// SetSettings saves new settings and starts what they ask for.
func (a *AI) SetSettings(s AISettings) AISettings {
	s = normalize(s)
	a.mu.Lock()
	before := a.settings
	a.settings = s
	if before.Model != s.Model {
		a.failed = map[string]bool{}
	}
	if before.Search != s.Search {
		a.searchErr = ""
	}
	a.unpauseLocked()
	a.mu.Unlock()
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		writeFileAtomic(a.settingsPath(), b)
	}
	a.sync()
	return s
}

// SetKey saves the API key in the Keychain, or removes it when key is "".
func (a *AI) SetKey(key string) error {
	key = strings.TrimSpace(key)
	var err error
	if key == "" {
		err = mygo.Secrets.Delete(keyName)
	} else {
		err = mygo.Secrets.Set(keyName, []byte(key))
	}
	if err != nil {
		return fmt.Errorf("couldn't save the key in the Keychain: %w", err)
	}
	a.mu.Lock()
	a.key, a.keyRead = key, true
	a.failed = map[string]bool{}
	a.searchErr = ""
	a.unpauseLocked()
	a.mu.Unlock()
	a.sync()
	return nil
}

// Test checks a key, or the saved one when key is "", by describing a
// tiny image with the model given. It returns the description.
func (a *AI) Test(ctx context.Context, key string, model string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = a.apiKey()
	}
	if key == "" {
		return "", errors.New("enter an API key first")
	}
	s := normalize(AISettings{Model: model})
	text, err := a.describeImage(ctx, key, s, testImage())
	if err != nil {
		return "", friendly(err, goService)
	}
	return text, nil
}

// Describe describes images, again when again is true, ahead of those
// waiting.
func (a *AI) Describe(ids []string, again bool) error {
	if a.apiKey() == "" {
		return errors.New("add an OpenCode API key in AI Settings first")
	}
	a.mu.Lock()
	a.unpauseLocked()
	var front []string
	for _, id := range ids {
		delete(a.failed, id)
		if again {
			a.redo[id] = true
		}
		if a.working[id] {
			continue
		}
		front = append(front, id)
		a.queued[id] = true
	}
	rest := a.queue[:0]
	for _, id := range a.queue {
		if !contains(front, id) {
			rest = append(rest, id)
		}
	}
	a.queue = append(front, rest...)
	a.mu.Unlock()
	a.pump()
	a.changed()
	return nil
}

// Resume starts describing again after it paused.
func (a *AI) Resume() {
	a.mu.Lock()
	a.unpauseLocked()
	a.fails = 0
	a.mu.Unlock()
	a.sync()
}

// Search returns the images whose descriptions and captions are closest
// in meaning to query, best first. It returns none while search by
// meaning is off, and pages then match words only.
func (a *AI) Search(ctx context.Context, query string) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	key := a.apiKey()
	a.mu.Lock()
	on := a.settings.Search && key != "" && a.searchErr == "" && len(a.vecs) > 0
	q := a.queryVecs[strings.ToLower(query)]
	a.mu.Unlock()
	if !on || query == "" {
		return nil, nil
	}
	if q == nil {
		vs, err := a.embed(ctx, key, []string{query}, true)
		if err != nil {
			a.searchFailed(err)
			return nil, friendly(err, zenService)
		}
		q = vs[0]
		a.mu.Lock()
		if len(a.queryVecs) > 200 {
			a.queryVecs = map[string][]float32{}
		}
		a.queryVecs[strings.ToLower(query)] = q
		a.mu.Unlock()
	}
	texts := a.lib.searchTexts()
	a.mu.Lock()
	hits := make([]SearchHit, 0, 256)
	for id, text := range texts {
		v := a.vecs[embedKey(text)]
		if v == nil {
			continue
		}
		hits = append(hits, SearchHit{ID: id, Score: dot(q, v)})
	}
	a.mu.Unlock()
	return rank(hits), nil
}

// rank sorts hits, one for every image with a vector, best first and keeps
// those that stand out. How close unrelated images come varies with the
// query ("a cat" scores 0.30 against images without one, as high as the
// right image for some queries), so a hit must beat the library's median
// for the query by a margin, unless it scores high on its own, and stay
// near the best.
func rank(hits []SearchHit) []SearchHit {
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) == 0 {
		return hits
	}
	best := hits[0].Score
	median := hits[len(hits)/2].Score
	cut := max(minScore, min(median+aboveMedian, highScore), best-scoreSpread)
	n := 0
	for n < len(hits) && n < maxHits && hits[n].Score >= cut {
		n++
	}
	return hits[:n]
}

// These suit text-embedding-3-large cut to 512 dimensions; other models
// score on other scales.
const (
	minScore    = 0.25
	aboveMedian = 0.07
	highScore   = 0.40
	scoreSpread = 0.15
	maxHits     = 600
)

// ---- describing ----

// sync queues what the settings ask for: images without a description
// when describing is automatic, and texts without vectors.
func (a *AI) sync() {
	key := a.apiKey()
	items := a.lib.aiItems()
	a.mu.Lock()
	s := a.settings
	if key != "" && s.Auto && a.paused == "" {
		for _, it := range items {
			if _, ok := a.desc[it.version]; ok || a.queued[it.id] || a.working[it.id] || a.failed[it.id] {
				continue
			}
			a.queue = append(a.queue, it.id)
			a.queued[it.id] = true
		}
	}
	embedOn := key != "" && s.Search && a.searchErr == ""
	if embedOn {
		for _, it := range items {
			text := it.searchText(a.desc[it.version].Text)
			if text == "" {
				continue
			}
			k := embedKey(text)
			if _, ok := a.vecs[k]; !ok {
				a.embedWait[k] = text
			}
		}
	}
	a.mu.Unlock()
	a.pump()
	if embedOn {
		a.scheduleEmbed()
	}
	a.changed()
}

// pump starts workers while there is work and room for it.
func (a *AI) pump() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for a.running < workers && len(a.queue) > 0 && a.paused == "" && a.key != "" {
		a.running++
		go a.work()
	}
}

func (a *AI) work() {
	for {
		a.mu.Lock()
		if len(a.queue) == 0 || a.paused != "" || a.key == "" {
			a.running--
			a.mu.Unlock()
			a.changed()
			return
		}
		id := a.queue[0]
		a.queue = a.queue[1:]
		delete(a.queued, id)
		redo := a.redo[id]
		delete(a.redo, id)
		key, s := a.key, a.settings
		a.working[id] = true
		a.mu.Unlock()
		a.changed()

		err := a.describeOne(key, s, id, redo)

		a.mu.Lock()
		delete(a.working, id)
		if err == nil {
			a.fails = 0
			a.mu.Unlock()
			a.changed()
			continue
		}
		var ae *apiError
		isAPI := errors.As(err, &ae)
		switch {
		case isAPI && (ae.status == 401 || ae.status == 402 || ae.status == 403):
			a.pauseLocked(friendly(err, goService).Error(), 0)
			a.requeueLocked(id, redo)
		case isAPI && ae.status == 429:
			// Go's allowance ran out: wait for it, however long that is.
			wait := ae.retryAfter
			if wait <= 0 {
				a.mu.Unlock()
				wait = a.goResetIn(key)
				a.mu.Lock()
			}
			at := time.Now().Add(wait)
			when := at.Format("15:04")
			if wait > 20*time.Hour {
				when = at.Format("Mon 2 Jan, 15:04")
			}
			a.pauseLocked(fmt.Sprintf("Your OpenCode Go allowance ran out. Describing goes on at %s, when it resets.", when), wait)
			a.requeueLocked(id, redo)
		default:
			// This image failed; others may work. Many failures in a row
			// pause describing below.
			a.failed[id] = true
			a.lastErr = friendly(err, goService).Error()
			a.fails++
		}
		if a.fails >= 5 && a.paused == "" {
			a.pauseLocked(a.lastErr, 0)
			// Give the images that failed another try when it resumes.
			a.failed = map[string]bool{}
		}
		a.mu.Unlock()
		a.changed()
	}
}

func (a *AI) requeueLocked(id string, redo bool) {
	if !a.queued[id] {
		a.queue = append([]string{id}, a.queue...)
		a.queued[id] = true
	}
	if redo {
		a.redo[id] = true
	}
}

// pauseLocked stops describing for why, and starts it again after wait
// unless wait is 0.
func (a *AI) pauseLocked(why string, wait time.Duration) {
	a.paused = why
	if a.resume != nil {
		a.resume.Stop()
		a.resume = nil
	}
	a.resumes = time.Time{}
	if wait > 0 {
		a.resume = time.AfterFunc(wait, a.Resume)
		a.resumes = time.Now().Add(wait)
	}
}

func (a *AI) unpauseLocked() {
	a.paused = ""
	a.lastErr = ""
	a.resumes = time.Time{}
	if a.resume != nil {
		a.resume.Stop()
		a.resume = nil
	}
}

// describeOne describes an image and keeps what the model wrote.
func (a *AI) describeOne(key string, s AISettings, id string, redo bool) error {
	img, ok := a.lib.imageCopy(id)
	if !ok {
		return nil
	}
	a.mu.Lock()
	_, done := a.desc[img.Version]
	a.mu.Unlock()
	if done && !redo {
		return nil
	}
	thumb, err := a.lib.ensureThumb(&img)
	if err != nil {
		return fmt.Errorf("no thumbnail of %s: %w", img.Name, err)
	}
	data, err := os.ReadFile(thumb)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	text, err := a.describeImage(ctx, key, s, data)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.desc[img.Version] = savedDesc{Text: text, Model: s.Model}
	if s.Search && a.searchErr == "" {
		st := img.searchText(text)
		a.embedWait[embedKey(st)] = st
	}
	a.mu.Unlock()
	a.saveDescriptions()
	a.scheduleEmbed()
	if a.lib.setDescription(id, img.Version, text) {
		a.emit(Description{ID: id, Description: text})
	}
	return nil
}

// describeImage asks the model to describe a JPEG.
func (a *AI) describeImage(ctx context.Context, key string, s AISettings, jpeg []byte) (string, error) {
	prompt := fmt.Sprintf(describePrompt, s.Language)
	body := map[string]any{
		"model": s.Model,
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg)}},
				map[string]any{"type": "text", "text": prompt},
			}},
		},
	}
	var res struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := a.post(ctx, key, a.goURL+"/chat/completions", body, &res); err != nil {
		return "", err
	}
	if len(res.Choices) == 0 {
		return "", errors.New("the model sent no description")
	}
	text := cleanDescription(contentText(res.Choices[0].Message.Content))
	if text == "" {
		return "", errors.New("the model sent an empty description")
	}
	return text, nil
}

const describePrompt = `Describe this image so it can be found by searching for what it shows.

Write 2 to 4 plain sentences: the subject, the setting, notable objects and people (by appearance, not identity), any text that is visible, the colours, the kind of image (photo, screenshot, illustration, diagram, meme…) and its mood. Then write one last line that starts with "Keywords:" and lists 8 to 15 comma-separated search terms, synonyms included.

Write in %s. Don't add a title, a preamble or markdown.`

// goResetIn asks Go when the allowance that ran out comes back: the latest
// reset of its limits that are reached. Without an answer it guesses ten
// minutes, and tries again then.
func (a *AI) goResetIn(key string) time.Duration {
	const guess = 10 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var res struct {
		Usage map[string]struct {
			Status   string          `json:"status"`
			ResetsAt json.RawMessage `json:"resetsAt"`
		} `json:"usage"`
	}
	if err := a.get(ctx, key, a.goUsageURL, &res); err != nil {
		log.Printf("eagle: Go usage: %v", err)
		return guess
	}
	var latest time.Time
	for _, u := range res.Usage {
		if u.Status != "rate-limited" {
			continue
		}
		if t, ok := parseTime(u.ResetsAt); ok && t.After(latest) {
			latest = t
		}
	}
	if latest.IsZero() {
		return guess
	}
	// A moment after the reset, so the first request falls after it.
	return max(time.Until(latest)+30*time.Second, time.Minute)
}

// parseTime reads a time sent as RFC 3339 text or as Unix seconds or
// milliseconds.
func parseTime(raw json.RawMessage) (time.Time, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			raw = json.RawMessage(strconv.FormatInt(n, 10))
		} else {
			return time.Time{}, false
		}
	}
	var n float64
	if json.Unmarshal(raw, &n) != nil || n <= 0 {
		return time.Time{}, false
	}
	if n > 1e12 {
		return time.UnixMilli(int64(n)), true
	}
	return time.Unix(int64(n), 0), true
}

// contentText reads a message's content, a string or a list of parts.
func contentText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" || p.Type == "" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

var (
	thinkRE = regexp.MustCompile(`(?s)<think>.*?</think>`)
	// keywordsRE finds the label of the keywords line, which models
	// sometimes run on at the end of the sentences.
	keywordsRE = regexp.MustCompile(`(?i)\s*\b(keywords|key words)\s*[:：]\s*|\s*(关键词|關鍵詞|キーワード)\s*[:：]\s*`)
)

// cleanDescription takes out what models add around a description:
// their reasoning, markdown and labels.
func cleanDescription(s string) string {
	s = thinkRE.ReplaceAllString(s, "")
	var lines []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		ln = strings.TrimLeft(ln, "#*> ")
		ln = strings.ReplaceAll(ln, "**", "")
		if ln == "" {
			continue
		}
		if l := strings.ToLower(ln); strings.HasPrefix(l, "description:") {
			ln = strings.TrimSpace(ln[len("description:"):])
		}
		lines = append(lines, ln)
	}
	s = strings.Join(lines, "\n")
	if loc := keywordsRE.FindStringSubmatchIndex(s); loc != nil {
		g := 2 // the group that matched: the English label or another
		if loc[g] < 0 {
			g = 4
		}
		label := s[loc[g]:loc[g+1]]
		s = strings.TrimSpace(strings.TrimSpace(s[:loc[0]]) + "\n" + label + ": " + strings.TrimSpace(s[loc[1]:]))
	}
	if r := []rune(s); len(r) > 2000 {
		s = string(r[:2000])
	}
	return s
}

// ---- embedding ----

func embedKey(text string) string { return shortHash(embedModel + "\x00" + text) }

func (a *AI) scheduleEmbed() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.embedTimer == nil && !a.embedBusy && len(a.embedWait) > 0 {
		a.embedTimer = time.AfterFunc(1500*time.Millisecond, a.embedPending)
	}
}

// embedPending embeds the texts waiting for vectors, a batch at a time.
func (a *AI) embedPending() {
	a.mu.Lock()
	a.embedTimer = nil
	if a.embedBusy {
		a.mu.Unlock()
		return
	}
	a.embedBusy = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.embedBusy = false
		a.mu.Unlock()
	}()
	for {
		a.mu.Lock()
		if !a.settings.Search || a.searchErr != "" || a.key == "" || len(a.embedWait) == 0 {
			a.mu.Unlock()
			return
		}
		key := a.key
		var keys, texts []string
		for k, t := range a.embedWait {
			keys = append(keys, k)
			texts = append(texts, t)
			if len(keys) == 64 {
				break
			}
		}
		a.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		vs, err := a.embed(ctx, key, texts, false)
		cancel()
		if err != nil {
			a.searchFailed(err)
			return
		}
		a.mu.Lock()
		for i, k := range keys {
			a.vecs[k] = vs[i]
			delete(a.embedWait, k)
		}
		a.mu.Unlock()
		a.saveVectors()
		a.changed()
	}
}

// searchFailed turns search by meaning off for this session after Zen
// refused to embed, or says why it failed for now.
func (a *AI) searchFailed(err error) {
	log.Printf("eagle: embedding: %v", err)
	var ae *apiError
	a.mu.Lock()
	if errors.As(err, &ae) && ae.status >= 400 && ae.status < 500 && ae.status != 429 {
		a.searchErr = friendly(err, zenService).Error()
	} else {
		a.lastErr = "Search by meaning: " + friendly(err, zenService).Error()
		// Try again later.
		if a.embedTimer == nil {
			a.embedTimer = time.AfterFunc(time.Minute, a.embedPending)
		}
	}
	a.mu.Unlock()
	a.changed()
}

// embed returns unit vectors of texts, from Zen: of search queries when
// query is true, of what they search otherwise.
func (a *AI) embed(ctx context.Context, key string, texts []string, query bool) ([][]float32, error) {
	if query && strings.HasPrefix(embedModel, "qwen3-embedding") {
		// Qwen's embeddings take an instruction with queries.
		q := make([]string, len(texts))
		for i, t := range texts {
			q[i] = "Instruct: Given a description of an image someone is looking for, find descriptions of matching images\nQuery: " + t
		}
		texts = q
	}
	body := map[string]any{"model": embedModel, "input": texts}
	var res struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := a.post(ctx, key, a.zenURL+"/embeddings", body, &res); err != nil {
		return nil, err
	}
	out := make([][]float32, len(texts))
	for i, d := range res.Data {
		idx := d.Index
		if idx < 0 || idx >= len(out) || out[idx] != nil {
			idx = i
		}
		if idx < len(out) {
			out[idx] = unit(d.Embedding)
		}
	}
	for _, v := range out {
		if v == nil {
			return nil, errors.New("OpenCode sent fewer vectors than texts")
		}
	}
	return out, nil
}

// unit cuts v to embedDims and scales it to length 1.
func unit(v []float64) []float32 {
	if len(v) > embedDims {
		v = v[:embedDims]
	}
	var n float64
	for _, x := range v {
		n += x * x
	}
	n = math.Sqrt(n)
	out := make([]float32, len(v))
	for i, x := range v {
		if n > 0 {
			out[i] = float32(x / n)
		}
	}
	return out
}

func dot(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// ---- HTTP ----

// apiError is a response of OpenCode other than 200.
type apiError struct {
	status     int
	message    string
	retryAfter time.Duration
}

func (e *apiError) Error() string {
	if e.message == "" {
		return fmt.Sprintf("OpenCode answered %d", e.status)
	}
	return fmt.Sprintf("OpenCode answered %d: %s", e.status, e.message)
}

func (a *AI) post(ctx context.Context, key, url string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return a.do(req, key, out)
}

func (a *AI) get(ctx context.Context, key, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return a.do(req, key, out)
}

func (a *AI) do(req *http.Request, key string, out any) error {
	req.Header.Set("Authorization", "Bearer "+key)
	// Go serves apps that name themselves and send a session ID.
	req.Header.Set("User-Agent", "Eagle/0.1 (image library)")
	req.Header.Set("x-opencode-session", a.session)
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		e := &apiError{status: res.StatusCode, message: errorText(data)}
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s > 0 {
			e.retryAfter = time.Duration(s) * time.Second
		}
		return e
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("OpenCode sent a response Eagle can't read: %w", err)
	}
	return nil
}

// errorText finds the message of an error response.
func errorText(data []byte) string {
	var v struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
		Type    string          `json:"type"`
	}
	if json.Unmarshal(data, &v) == nil {
		var e struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		}
		var s string
		switch {
		case json.Unmarshal(v.Error, &e) == nil && e.Message != "":
			return e.Message
		case json.Unmarshal(v.Error, &s) == nil && s != "":
			return s
		case v.Message != "":
			return v.Message
		}
	}
	s := strings.TrimSpace(string(data))
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return s
}

// friendly says what an error means for the user.
func friendly(err error, svc service) error {
	var ae *apiError
	if !errors.As(err, &ae) {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("OpenCode took too long to answer")
		}
		return fmt.Errorf("couldn't reach OpenCode: %v", err)
	}
	msg := strings.ToLower(ae.message)
	switch {
	case ae.status == 401:
		return errors.New("OpenCode didn't accept the API key")
	case ae.status == 403 && strings.Contains(msg, "model access"):
		return errors.New("this model is turned off for your OpenCode workspace; turn it on in the OpenCode console, or pick another model")
	case ae.status == 403 && strings.Contains(msg, "region"):
		return fmt.Errorf("this model is hosted in a region your OpenCode workspace doesn't allow; allow it in the console's privacy settings, or pick another model (%s)", ae.message)
	case ae.status == 403 && svc == goService && (strings.Contains(msg, "entitlement") || strings.Contains(msg, "subscription")):
		return errors.New("this API key has no OpenCode Go subscription")
	case strings.Contains(msg, "balance") || strings.Contains(msg, "insufficient") || strings.Contains(msg, "credit"):
		return errors.New("the OpenCode Zen balance has run out; add credit in the OpenCode console")
	case ae.status == 429 && svc == goService:
		return errors.New("your OpenCode Go allowance ran out for now")
	case ae.status == 429:
		return errors.New("OpenCode's usage limit was reached")
	}
	return ae
}

// ---- saving and telling pages ----

func (a *AI) saveDescriptions() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.saveTimer != nil {
		a.saveTimer.Stop()
	}
	a.saveTimer = time.AfterFunc(time.Second, func() {
		a.mu.Lock()
		b, err := json.Marshal(a.desc)
		a.mu.Unlock()
		if err == nil {
			writeFileAtomic(a.descPath(), b)
		}
	})
}

func (a *AI) saveVectors() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vecTimer != nil {
		a.vecTimer.Stop()
	}
	a.vecTimer = time.AfterFunc(2*time.Second, func() {
		var buf bytes.Buffer
		a.mu.Lock()
		err := gob.NewEncoder(&buf).Encode(vectorFile{Model: embedModel, Dims: embedDims, Vecs: a.vecs})
		a.mu.Unlock()
		if err == nil {
			writeFileAtomic(a.vecPath(), buf.Bytes())
		}
	})
}

func (a *AI) emit(d Description) {
	a.outMu.Lock()
	defer a.outMu.Unlock()
	a.out = append(a.out, d)
	if a.outTimer == nil {
		a.outTimer = time.AfterFunc(300*time.Millisecond, func() {
			a.outMu.Lock()
			batch := a.out
			a.out = nil
			a.outTimer = nil
			a.outMu.Unlock()
			_ = DescriptionsChanged.Broadcast(batch)
		})
	}
}

// changed sends the status to pages, at most a few times a second.
func (a *AI) changed() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.statusTimer == nil {
		a.statusTimer = time.AfterFunc(250*time.Millisecond, func() {
			a.mu.Lock()
			a.statusTimer = nil
			a.mu.Unlock()
			_ = AIStatusChanged.Broadcast(a.status())
		})
	}
}

func (a *AI) status() AIStatus {
	items := a.lib.aiItems()
	a.mu.Lock()
	defer a.mu.Unlock()
	st := AIStatus{
		Ready:       a.key != "",
		Auto:        a.settings.Auto,
		Search:      a.settings.Search && a.key != "" && a.searchErr == "",
		Total:       len(items),
		Pending:     len(a.queue) + len(a.working),
		Working:     make([]string, 0, len(a.working)),
		Paused:      a.paused,
		SearchError: a.searchErr,
		LastError:   a.lastErr,
	}
	if !a.settings.Search {
		st.SearchError = ""
	}
	if !a.resumes.IsZero() {
		st.ResumesAt = a.resumes.UnixMilli()
	}
	for id := range a.working {
		st.Working = append(st.Working, id)
	}
	sort.Strings(st.Working)
	for _, it := range items {
		if _, ok := a.desc[it.version]; ok {
			st.Described++
		}
	}
	return st
}

// descriptionOf returns the description of an image's version, "" without
// one.
func (a *AI) descriptionOf(version string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.desc[version].Text
}

// ---- what the library tells AI ----

type aiItem struct {
	id, version, caption string
}

// searchText is what is embedded for an image: its description and its
// caption.
func (it aiItem) searchText(desc string) string {
	return searchText(desc, it.caption)
}

func (img *Image) searchText(desc string) string { return searchText(desc, img.Caption) }

func searchText(desc, caption string) string {
	caption = strings.TrimSpace(caption)
	if r := []rune(caption); len(r) > 1000 {
		caption = string(r[:1000])
	}
	return strings.TrimSpace(strings.TrimSpace(desc) + "\n" + caption)
}

// aiItems lists the library's images for AI, in the order of folders.
func (l *Library) aiItems() []aiItem {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]aiItem, 0, len(l.images))
	for _, f := range l.folders {
		for _, id := range f.ids {
			if img := l.images[id]; img != nil {
				out = append(out, aiItem{id: img.ID, version: img.Version, caption: img.Caption})
			}
		}
	}
	return out
}

// searchTexts returns the embedded text of each image that has one.
func (l *Library) searchTexts() map[string]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]string, len(l.images))
	for id, img := range l.images {
		if t := img.searchText(img.Description); t != "" {
			out[id] = t
		}
	}
	return out
}

func (l *Library) imageCopy(id string) (Image, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	img := l.images[id]
	if img == nil {
		return Image{}, false
	}
	return *img, true
}

// setDescription gives an image its description, unless the file changed
// meanwhile, and tells whether it did.
func (l *Library) setDescription(id, version, text string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	img := l.images[id]
	if img == nil || img.Version != version {
		return false
	}
	img.Description = text
	return true
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// testImage is a small JPEG for Test: a red circle on white.
func testImage() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 96, 96))
	for y := range 96 {
		for x := range 96 {
			c := color.RGBA{255, 255, 255, 255}
			if dx, dy := x-48, y-48; dx*dx+dy*dy < 36*36 {
				c = color.RGBA{220, 40, 40, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}
