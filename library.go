package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// Folder is a folder the user added to the library. Its images stay where
// they are: Eagle only indexes them.
type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
	// Count is how many images the folder holds, its subfolders included.
	Count int `json:"count"`
	// Indexing tells how far thumbnails and palettes have come while the
	// folder is being indexed, and is null otherwise.
	Indexing *Progress `json:"indexing"`
	// Missing is true when the folder can't be read, as when its disk is
	// ejected.
	Missing bool `json:"missing"`
}

// Progress counts the images of a folder indexed so far.
type Progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// Image is an image file in one of the library's folders.
type Image struct {
	ID     string `json:"id"`
	Folder string `json:"folder"`
	// Dir is the image's subfolder in its folder, separated by slashes,
	// and "" for images at the top of the folder.
	Dir  string `json:"dir"`
	Name string `json:"name"`
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
	// Width and Height are the displayed size, 0 until they are known.
	Width  int   `json:"width"`
	Height int   `json:"height"`
	Size   int64 `json:"size"`
	// Modified is the file's modification time, in Unix milliseconds.
	Modified int64     `json:"modified"`
	Palette  []string  `json:"palette"`
	Weights  []float64 `json:"weights"`
	// Colors are the color families of the palette, for the color filter.
	Colors   []string `json:"colors"`
	Rating   int      `json:"rating"`
	Favorite bool     `json:"favorite"`
	Tags     []string `json:"tags"`
	// Version changes whenever the file does; image URLs carry it so the
	// page never shows a stale thumbnail.
	Version string `json:"version"`
}

// ImageUpdate is what indexing learned about an image after the folder was
// listed: its size, for formats only ImageIO reads, and its palette.
type ImageUpdate struct {
	ID      string    `json:"id"`
	Width   int       `json:"width"`
	Height  int       `json:"height"`
	Palette []string  `json:"palette"`
	Weights []float64 `json:"weights"`
	Colors  []string  `json:"colors"`
}

// FolderImages is every image of a folder, sent when it is listed again.
type FolderImages struct {
	Folder string  `json:"folder"`
	Images []Image `json:"images"`
}

// Snapshot is the whole library.
type Snapshot struct {
	Folders []Folder `json:"folders"`
	Images  []Image  `json:"images"`
}

// FoldersChanged is sent when folders are added, removed, recounted or make
// progress indexing.
var FoldersChanged = mygo.NewEvent[[]Folder]("folders")

// ImagesChanged is sent with every image of a folder each time it is
// listed: when it is added, and when files in it change.
var ImagesChanged = mygo.NewEvent[FolderImages]("images")

// ImagesUpdated is sent in batches while a folder is indexed.
var ImagesUpdated = mygo.NewEvent[[]ImageUpdate]("imageUpdates")

// FolderRemoved is sent with the ID of a folder taken out of the library,
// whose images go with it.
var FolderRemoved = mygo.NewEvent[string]("folderRemoved")

type meta struct {
	Rating   int      `json:"rating,omitempty"`
	Favorite bool     `json:"favorite,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

type cached struct {
	W       int       `json:"w"`
	H       int       `json:"h"`
	Palette []string  `json:"p,omitempty"`
	Weights []float64 `json:"wt,omitempty"`
}

type savedFolder struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type savedLibrary struct {
	Folders []savedFolder    `json:"folders"`
	Meta    map[string]*meta `json:"meta"`
}

type folderState struct {
	Folder
	ids    []string
	cancel context.CancelFunc
	rescan *time.Timer
}

type trashedFile struct {
	original string
	trashed  string
	folder   string
}

// Library is the user's image library: the folders they added, the images
// in them and what they said about each, its rating, tags and whether it
// is a favorite. Pages call its methods through src/mygo.ts.
type Library struct {
	mu       sync.Mutex
	dataDir  string
	cacheDir string
	folders  []*folderState
	images   map[string]*Image
	meta     map[string]*meta
	cache    map[string]*cached
	trashed  map[string]trashedFile
	pending  map[string]chan struct{}

	saveTimer  *time.Timer
	cacheTimer *time.Timer
	// work limits how many images indexing converts at once, and onDemand
	// how many thumbnails pages wait for, which go ahead of indexing.
	work     chan struct{}
	onDemand chan struct{}

	updMu   sync.Mutex
	updates []ImageUpdate
	updTime *time.Timer
}

func newLibrary() *Library {
	n := min(max(runtime.NumCPU()-2, 2), 6)
	return &Library{
		images:   map[string]*Image{},
		meta:     map[string]*meta{},
		cache:    map[string]*cached{},
		trashed:  map[string]trashedFile{},
		pending:  map[string]chan struct{}{},
		work:     make(chan struct{}, n),
		onDemand: make(chan struct{}, 3),
	}
}

// start loads the library and indexes its folders. It runs once the app is
// ready, never while `mygo generate` builds the app.
func (l *Library) start() {
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Printf("eagle: user data: %v", err)
		data = os.TempDir()
	}
	cache, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		cache = filepath.Join(data, "Cache")
	}
	l.dataDir, l.cacheDir = data, cache
	_ = os.MkdirAll(data, 0o755)
	_ = os.MkdirAll(cache, 0o755)

	var saved savedLibrary
	if b, err := os.ReadFile(filepath.Join(data, "library.json")); err == nil {
		_ = json.Unmarshal(b, &saved)
	}
	if b, err := os.ReadFile(filepath.Join(cache, "index.json")); err == nil {
		_ = json.Unmarshal(b, &l.cache)
	}
	l.mu.Lock()
	if saved.Meta != nil {
		l.meta = saved.Meta
	}
	for _, f := range saved.Folders {
		l.folders = append(l.folders, &folderState{Folder: Folder{ID: f.ID, Name: filepath.Base(f.Path), Path: f.Path}})
	}
	folders := append([]*folderState(nil), l.folders...)
	l.mu.Unlock()
	for _, f := range folders {
		go l.scan(f.ID)
	}
}

// ---- pages call these ----

// Snapshot returns the library as it is now.
func (l *Library) Snapshot() Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := Snapshot{Folders: l.foldersLocked(), Images: make([]Image, 0, len(l.images))}
	for _, f := range l.folders {
		for _, id := range f.ids {
			if img := l.images[id]; img != nil {
				s.Images = append(s.Images, *img)
			}
		}
	}
	return s
}

// AddFolder asks the user for folders and adds them to the library. It
// returns those added, none when the user canceled.
func (l *Library) AddFolder(ctx context.Context) ([]Folder, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      mygo.CallerWindow(ctx),
		Title:       "Add Folders",
		Message:     "Choose folders of images. They are indexed where they are; nothing is copied or moved.",
		ButtonLabel: "Add",
		Directory:   true,
		Multiple:    true,
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	return l.AddPaths(paths)
}

// AddPaths adds folders dropped on the window, or the folders of dropped
// files.
func (l *Library) AddPaths(paths []string) ([]Folder, error) {
	var added []Folder
	var errs []error
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		st, err := os.Stat(abs)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !st.IsDir() {
			abs = filepath.Dir(abs)
		}
		f, err := l.addFolder(abs)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		added = append(added, f)
	}
	if len(added) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return added, nil
}

// RemoveFolder takes a folder out of the library. Its files stay on disk.
func (l *Library) RemoveFolder(id string) error {
	l.mu.Lock()
	idx := -1
	for i, f := range l.folders {
		if f.ID == id {
			idx = i
		}
	}
	if idx < 0 {
		l.mu.Unlock()
		return errors.New("no such folder")
	}
	f := l.folders[idx]
	if f.cancel != nil {
		f.cancel()
	}
	for _, imgID := range f.ids {
		delete(l.images, imgID)
	}
	l.folders = append(l.folders[:idx], l.folders[idx+1:]...)
	folders := l.foldersLocked()
	l.saveLocked()
	l.mu.Unlock()
	_ = FolderRemoved.Broadcast(id)
	_ = FoldersChanged.Broadcast(folders)
	return nil
}

// Rescan lists a folder again and indexes what changed.
func (l *Library) Rescan(id string) {
	go l.scan(id)
}

// SetRating rates images from 0, unrated, to 5 stars.
func (l *Library) SetRating(ids []string, rating int) {
	rating = min(max(rating, 0), 5)
	l.editMeta(ids, func(m *meta, img *Image) { m.Rating = rating; img.Rating = rating })
}

// SetFavorite marks images as favorites, or not.
func (l *Library) SetFavorite(ids []string, favorite bool) {
	l.editMeta(ids, func(m *meta, img *Image) { m.Favorite = favorite; img.Favorite = favorite })
}

// AddTag tags images.
func (l *Library) AddTag(ids []string, tag string) {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return
	}
	l.editMeta(ids, func(m *meta, img *Image) {
		for _, t := range m.Tags {
			if t == tag {
				return
			}
		}
		m.Tags = append(m.Tags, tag)
		img.Tags = append([]string(nil), m.Tags...)
	})
}

// RemoveTag removes a tag from images.
func (l *Library) RemoveTag(ids []string, tag string) {
	l.editMeta(ids, func(m *meta, img *Image) {
		out := m.Tags[:0]
		for _, t := range m.Tags {
			if t != tag {
				out = append(out, t)
			}
		}
		m.Tags = out
		img.Tags = append([]string(nil), out...)
	})
}

// Reveal shows an image in Finder.
func (l *Library) Reveal(id string) error {
	p, err := l.pathOf(id)
	if err != nil {
		return err
	}
	mygo.Shell.ShowItemInFolder(p)
	return nil
}

// OpenFolder opens one of the library's folders in Finder.
func (l *Library) OpenFolder(id string) error {
	l.mu.Lock()
	f := l.folder(id)
	l.mu.Unlock()
	if f == nil {
		return errors.New("no such folder")
	}
	return mygo.Shell.OpenPath(f.Path)
}

// OpenExternally opens an image in the app the system opens it with.
func (l *Library) OpenExternally(id string) error {
	p, err := l.pathOf(id)
	if err != nil {
		return err
	}
	return mygo.Shell.OpenPath(p)
}

// QuickLook shows an image in the Quick Look panel.
func (l *Library) QuickLook(id string) error {
	p, err := l.pathOf(id)
	if err != nil {
		return err
	}
	return mygo.Shell.PreviewItem(p)
}

// Trash moves images to the Trash after the user confirms, and returns
// the IDs of those moved. Restore puts them back.
func (l *Library) Trash(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	msg := "Move 1 image to the Trash?"
	if len(ids) > 1 {
		msg = fmt.Sprintf("Move %d images to the Trash?", len(ids))
	}
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent:             mygo.CallerWindow(ctx),
		Type:               mygo.MessageWarning,
		Message:            msg,
		Detail:             "The files are moved to the Trash in Finder. You can undo this right after.",
		Buttons:            []string{"Move to Trash", "Cancel"},
		DefaultButton:      0,
		CancelButton:       1,
		DestructiveButtons: []int{0},
	})
	if err != nil || res.Button != 0 {
		return nil, err
	}
	var moved []string
	touched := map[string]bool{}
	var errs []error
	for _, id := range ids {
		l.mu.Lock()
		img := l.images[id]
		l.mu.Unlock()
		if img == nil {
			continue
		}
		where, err := mygo.Shell.TrashItemResult(img.Path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		l.mu.Lock()
		l.trashed[id] = trashedFile{original: img.Path, trashed: where, folder: img.Folder}
		l.mu.Unlock()
		moved = append(moved, id)
		touched[img.Folder] = true
	}
	for f := range touched {
		go l.scan(f)
	}
	return moved, errors.Join(errs...)
}

// Restore puts images Trash moved back where they were.
func (l *Library) Restore(ids []string) error {
	var errs []error
	touched := map[string]bool{}
	for _, id := range ids {
		l.mu.Lock()
		t, ok := l.trashed[id]
		delete(l.trashed, id)
		l.mu.Unlock()
		if !ok || t.trashed == "" {
			continue
		}
		if err := os.Rename(t.trashed, t.original); err != nil {
			errs = append(errs, err)
			continue
		}
		touched[t.folder] = true
	}
	for f := range touched {
		go l.scan(f)
	}
	return errors.Join(errs...)
}

// ---- folders ----

func folderID(path string) string { return shortHash(path) }

func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:8])
}

func (l *Library) addFolder(path string) (Folder, error) {
	path = filepath.Clean(path)
	l.mu.Lock()
	var absorbed []string
	for _, f := range l.folders {
		if f.Path == path || strings.HasPrefix(path, f.Path+string(filepath.Separator)) {
			l.mu.Unlock()
			return Folder{}, fmt.Errorf("%s is already in the library, in %s", filepath.Base(path), f.Name)
		}
		if strings.HasPrefix(f.Path, path+string(filepath.Separator)) {
			absorbed = append(absorbed, f.ID)
		}
	}
	l.mu.Unlock()
	// A folder that holds folders already in the library takes their place.
	for _, id := range absorbed {
		_ = l.RemoveFolder(id)
	}
	f := &folderState{Folder: Folder{ID: folderID(path), Name: filepath.Base(path), Path: path, Indexing: &Progress{}}}
	l.mu.Lock()
	l.folders = append(l.folders, f)
	l.saveLocked()
	folders := l.foldersLocked()
	l.mu.Unlock()
	_ = FoldersChanged.Broadcast(folders)
	go l.scan(f.ID)
	return f.Folder, nil
}

func (l *Library) folder(id string) *folderState {
	for _, f := range l.folders {
		if f.ID == id {
			return f
		}
	}
	return nil
}

func (l *Library) foldersLocked() []Folder {
	out := make([]Folder, len(l.folders))
	for i, f := range l.folders {
		out[i] = f.Folder
		if f.Indexing != nil {
			p := *f.Indexing
			out[i].Indexing = &p
		}
	}
	return out
}

func (l *Library) broadcastFolders() {
	l.mu.Lock()
	folders := l.foldersLocked()
	l.mu.Unlock()
	_ = FoldersChanged.Broadcast(folders)
}

var skipDirExt = map[string]bool{".app": true, ".photoslibrary": true, ".photolibrary": true, ".aplibrary": true, ".lrdata": true, ".bundle": true}

type found struct {
	path, rel string
	size      int64
	mod       time.Time
	kind      Kind
}

// scan lists a folder's images, sends them to pages, then indexes those
// without a thumbnail or palette and watches the folder for changes.
func (l *Library) scan(id string) {
	l.mu.Lock()
	f := l.folder(id)
	if f == nil {
		l.mu.Unlock()
		return
	}
	if f.cancel != nil {
		f.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	root := f.Path
	l.mu.Unlock()

	var files []found
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil
		}
		name := d.Name()
		if p != root && strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDirExt[strings.ToLower(filepath.Ext(name))] {
				return filepath.SkipDir
			}
			return nil
		}
		kind, ok := kindByExt[strings.ToLower(filepath.Ext(name))]
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		files = append(files, found{path: p, rel: filepath.ToSlash(rel), size: info.Size(), mod: info.ModTime(), kind: kind})
		return ctx.Err()
	})
	if ctx.Err() != nil {
		return
	}

	images := make([]*Image, len(files))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, fl := range files {
		dir := filepath.ToSlash(filepath.Dir(fl.rel))
		if dir == "." {
			dir = ""
		}
		img := &Image{
			ID:       shortHash(fl.path),
			Folder:   id,
			Dir:      dir,
			Name:     filepath.Base(fl.path),
			Path:     fl.path,
			Kind:     fl.kind,
			Size:     fl.size,
			Modified: fl.mod.UnixMilli(),
			Version:  shortHash(fmt.Sprintf("%s|%d|%d", fl.path, fl.size, fl.mod.UnixNano())),
			Palette:  []string{},
			Weights:  []float64{},
			Colors:   []string{},
			Tags:     []string{},
		}
		images[i] = img
		l.mu.Lock()
		c := l.cache[img.Version]
		if m := l.meta[img.ID]; m != nil {
			img.Rating, img.Favorite = m.Rating, m.Favorite
			img.Tags = append([]string{}, m.Tags...)
		}
		l.mu.Unlock()
		if c != nil {
			l.applyCache(img, c)
			continue
		}
		if !goDecodable[fl.kind] {
			continue // ImageIO reads its size while indexing
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if w, h, err := readDimensions(img.Path, img.Kind); err == nil {
				img.Width, img.Height = w, h
			}
		}()
	}
	wg.Wait()
	if ctx.Err() != nil {
		return
	}

	var todo []*Image
	l.mu.Lock()
	f = l.folder(id)
	if f == nil {
		l.mu.Unlock()
		return
	}
	for _, old := range f.ids {
		delete(l.images, old)
	}
	f.ids = f.ids[:0]
	list := make([]Image, len(images))
	for i, img := range images {
		l.images[img.ID] = img
		f.ids = append(f.ids, img.ID)
		list[i] = *img
		if img.Width == 0 || len(img.Palette) == 0 || !fileExists(l.thumbPath(img)) {
			todo = append(todo, img)
		}
	}
	f.Count = len(images)
	f.Missing = err != nil
	if len(todo) > 0 {
		f.Indexing = &Progress{Total: len(todo)}
	} else {
		f.Indexing = nil
	}
	folders := l.foldersLocked()
	l.mu.Unlock()
	_ = ImagesChanged.Broadcast(FolderImages{Folder: id, Images: list})
	_ = FoldersChanged.Broadcast(folders)

	go l.watch(ctx, id, root)
	if len(todo) > 0 {
		l.index(ctx, id, todo)
	}
}

// index makes thumbnails and palettes of images, a few at a time.
func (l *Library) index(ctx context.Context, folder string, todo []*Image) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	last := time.Now()
	for _, img := range todo {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case l.work <- struct{}{}:
		}
		wg.Add(1)
		go func(img *Image) {
			defer wg.Done()
			defer func() { <-l.work }()
			l.indexOne(img)
			mu.Lock()
			done++
			n := done
			report := time.Since(last) > 200*time.Millisecond || n == len(todo)
			if report {
				last = time.Now()
			}
			mu.Unlock()
			if report {
				l.mu.Lock()
				if f := l.folder(folder); f != nil && f.Indexing != nil {
					f.Indexing.Done = n
				}
				l.mu.Unlock()
				l.broadcastFolders()
			}
		}(img)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return
	}
	l.mu.Lock()
	if f := l.folder(folder); f != nil {
		f.Indexing = nil
	}
	l.mu.Unlock()
	l.flushUpdates()
	l.broadcastFolders()
	l.saveCache()
}

func (l *Library) indexOne(img *Image) {
	thumb, err := l.ensureThumb(img)
	if err != nil {
		return
	}
	w, h := img.Width, img.Height
	if w == 0 || h == 0 {
		if dw, dh, err := sipsDimensions(img.Path); err == nil {
			w, h = dw, dh
		}
	}
	var palette []Swatch
	if f, err := os.Open(thumb); err == nil {
		palette, _ = extractPalette(f)
		f.Close()
	}
	c := &cached{W: w, H: h}
	for _, s := range palette {
		c.Palette = append(c.Palette, s.Hex)
		c.Weights = append(c.Weights, s.Weight)
	}
	l.mu.Lock()
	l.cache[img.Version] = c
	l.applyCache(img, c)
	u := ImageUpdate{ID: img.ID, Width: img.Width, Height: img.Height, Palette: img.Palette, Weights: img.Weights, Colors: img.Colors}
	l.mu.Unlock()
	l.queueUpdate(u)
}

func (l *Library) applyCache(img *Image, c *cached) {
	if c.W > 0 {
		img.Width, img.Height = c.W, c.H
	}
	if len(c.Palette) > 0 {
		img.Palette = c.Palette
		img.Weights = c.Weights
		sw := make([]Swatch, len(c.Palette))
		for i := range c.Palette {
			sw[i] = Swatch{Hex: c.Palette[i]}
			if i < len(c.Weights) {
				sw[i].Weight = c.Weights[i]
			}
		}
		img.Colors = colorFamilies(sw)
	}
}

func (l *Library) queueUpdate(u ImageUpdate) {
	l.updMu.Lock()
	l.updates = append(l.updates, u)
	if l.updTime == nil {
		l.updTime = time.AfterFunc(250*time.Millisecond, l.flushUpdates)
	}
	l.updMu.Unlock()
}

func (l *Library) flushUpdates() {
	l.updMu.Lock()
	batch := l.updates
	l.updates = nil
	if l.updTime != nil {
		l.updTime.Stop()
		l.updTime = nil
	}
	l.updMu.Unlock()
	if len(batch) > 0 {
		_ = ImagesUpdated.Broadcast(batch)
	}
}

// watch lists the folder again when files in it change.
func (l *Library) watch(ctx context.Context, id, root string) {
	w, err := mygo.WatchFiles(ctx, root, mygo.FileWatchOptions{
		Recursive: true,
		Debounce:  400 * time.Millisecond,
		Ignore: func(p string, isDir bool) bool {
			base := p
			if i := strings.LastIndexByte(p, '/'); i >= 0 {
				base = p[i+1:]
			}
			if strings.HasPrefix(base, ".") {
				return true
			}
			if isDir {
				return skipDirExt[strings.ToLower(filepath.Ext(base))]
			}
			_, ok := kindByExt[strings.ToLower(filepath.Ext(base))]
			return !ok
		},
	})
	if err != nil {
		log.Printf("eagle: not watching %s: %v", root, err)
		return
	}
	defer w.Close()
	events, err := w.Next(ctx)
	if err != nil || len(events) == 0 {
		return
	}
	// The next scan starts a new watch, so this one ends here.
	go l.scan(id)
}

// ---- metadata ----

func (l *Library) editMeta(ids []string, fn func(*meta, *Image)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, id := range ids {
		img := l.images[id]
		if img == nil {
			continue
		}
		m := l.meta[id]
		if m == nil {
			m = &meta{}
			l.meta[id] = m
		}
		fn(m, img)
		if m.Rating == 0 && !m.Favorite && len(m.Tags) == 0 {
			delete(l.meta, id)
		}
	}
	l.saveLocked()
}

func (l *Library) pathOf(id string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	img := l.images[id]
	if img == nil {
		return "", errors.New("no such image")
	}
	return img.Path, nil
}

func (l *Library) saveLocked() {
	if l.dataDir == "" {
		return
	}
	if l.saveTimer != nil {
		l.saveTimer.Stop()
	}
	l.saveTimer = time.AfterFunc(300*time.Millisecond, func() {
		l.mu.Lock()
		s := savedLibrary{Meta: l.meta}
		for _, f := range l.folders {
			s.Folders = append(s.Folders, savedFolder{ID: f.ID, Path: f.Path})
		}
		b, err := json.MarshalIndent(s, "", "  ")
		l.mu.Unlock()
		if err == nil {
			writeFileAtomic(filepath.Join(l.dataDir, "library.json"), b)
		}
	})
}

func (l *Library) saveCache() {
	l.mu.Lock()
	if l.cacheTimer != nil {
		l.cacheTimer.Stop()
	}
	l.cacheTimer = time.AfterFunc(time.Second, func() {
		l.mu.Lock()
		b, err := json.Marshal(l.cache)
		l.mu.Unlock()
		if err == nil {
			writeFileAtomic(filepath.Join(l.cacheDir, "index.json"), b)
		}
	})
	l.mu.Unlock()
}

func writeFileAtomic(path string, b []byte) {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Printf("eagle: save %s: %v", path, err)
		return
	}
	_ = os.Rename(tmp, path)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ---- thumbnails and previews ----

func (l *Library) thumbPath(img *Image) string {
	return filepath.Join(l.cacheDir, "thumbs", img.Version[:2], img.Version+".jpg")
}

func (l *Library) previewPath(img *Image) string {
	return filepath.Join(l.cacheDir, "previews", img.Version+".jpg")
}

// ensureThumb returns the thumbnail of img, making it first if needed. Two
// callers asking for the same one wait for the same work.
func (l *Library) ensureThumb(img *Image) (string, error) {
	return l.ensure(l.thumbPath(img), img, thumbSize)
}

func (l *Library) ensure(dst string, img *Image, size int) (string, error) {
	if fileExists(dst) {
		return dst, nil
	}
	l.mu.Lock()
	if ch, ok := l.pending[dst]; ok {
		l.mu.Unlock()
		<-ch
		if fileExists(dst) {
			return dst, nil
		}
		return "", errors.New("could not make a thumbnail")
	}
	ch := make(chan struct{})
	l.pending[dst] = ch
	l.mu.Unlock()
	err := makeJPEG(img.Path, dst, img.Kind, size)
	l.mu.Lock()
	delete(l.pending, dst)
	l.mu.Unlock()
	close(ch)
	if err != nil {
		return "", err
	}
	return dst, nil
}

// mediaHandler serves the eagle: scheme: eagle://localhost/thumb/<id> is an
// image's thumbnail and eagle://localhost/full/<id> the image itself, or a
// large JPEG preview of formats pages can't show. It is its own type, as
// every exported method of Library is callable from pages.
type mediaHandler struct{ l *Library }

func (h mediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l := h.l
	what, id, _ := strings.Cut(strings.Trim(r.URL.Path, "/"), "/")
	l.mu.Lock()
	img := l.images[id]
	var cp Image
	if img != nil {
		cp = *img
	}
	l.mu.Unlock()
	if img == nil {
		http.NotFound(w, r)
		return
	}
	var file string
	var err error
	switch what {
	case "thumb":
		// A few at a time, so scrolling a large folder doesn't start
		// hundreds of conversions.
		l.onDemand <- struct{}{}
		file, err = l.ensureThumb(&cp)
		<-l.onDemand
	case "full":
		if webSafe[cp.Kind] {
			file = cp.Path
		} else {
			file, err = l.ensure(l.previewPath(&cp), &cp, previewSize)
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f, err := os.Open(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "max-age=31536000, immutable")
	http.ServeContent(w, r, filepath.Base(file), st.ModTime(), f)
}
