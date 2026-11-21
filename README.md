<p align="center">
  <img src="resources/icon.png" width="128" height="128" alt="Eagle icon">
</p>

<h1 align="center">Eagle</h1>

<p align="center">A fast, quiet image viewer for macOS.<br>Add your folders, then browse, filter and look closer, without copying a single file.</p>

![Eagle showing a library of images, with the colour filter bar and the inspector](docs/screenshot.png)

## What it does

Eagle indexes folders where they are. Nothing is imported, copied or moved, and new files show up as soon as they land in a folder you've added.

- **Library.** Browse in a justified layout that keeps each image's shape, or in an even grid. Images are grouped by date, and the thumbnail size is adjustable. Large libraries stay smooth because only what's on screen is drawn.
- **Sidebar.** All Images, Recent (modified in the last 30 days), Favorites and Untagged; each folder you've added, with its subfolders; and your tags.
- **Filters.** Filter by colour, file type, shape (landscape, portrait, square, panorama), resolution, rating and date. Search matches file names, tags and folder names as you type. Every image gets a five-colour palette when it's indexed, which is what colour filtering uses.
- **Inspector.** For one image: its palette, rating, favourite, tags and file details. For several: their shared palette, and rating and tagging for all of them at once. With nothing selected: a summary of what's in view.
- **Viewer.** Full-window viewing with fit and 1:1 zoom, pinch to zoom, drag to pan, a filmstrip and an info panel.
- **File handling.** Quick Look, Show in Finder, Open with the default app, and Move to Trash with Undo.

Supported formats are JPEG, PNG, GIF, WebP, HEIC, TIFF, BMP, AVIF and camera RAW (ARW, CR2, CR3, NEF, RAF, DNG, ORF, RW2, PEF, SRW). Formats a web view can't show are converted to JPEG previews with macOS's `sips`.

## Keyboard

| Keys | Action |
| --- | --- |
| ⌘O | Add a folder |
| ⌘F | Search |
| ← → ↑ ↓ | Move between images; with ⇧, extend the selection |
| ⌘A | Select all |
| Space or Return | Open in the viewer |
| 1–5 / 0 | Rate / clear the rating |
| F | Favourite |
| ⌘Y | Quick Look |
| ⇧⌘R | Show in Finder |
| ⌘⌫ | Move to Trash |
| ⌘1 / ⌘2 | Justified / grid layout |
| ⌘= / ⌘− | Larger / smaller thumbnails |
| ⇧⌘F | Show or hide the filter bar |
| ⌘I | Show or hide the inspector |

In the viewer: ← → move between images, + − zoom, 0 fits the image to the window, 1 shows it at actual size, I toggles the info panel, and Esc or Space closes it.

⌘-click or ⇧-click a colour swatch to filter by several colours at once.

## Development

Eagle is built with [MyGo](https://github.com/egoist/mygo): a Go backend with a TypeScript and Vite frontend in the system web view. It needs macOS, [Go](https://go.dev) 1.27 or later and [Bun](https://bun.sh).

```bash
bun install
```

```bash
bun run dev
```

`bun run dev` builds the app, starts Vite and rebuilds whenever a file changes. Run it in an interactive terminal, because `mygo dev` stops the app when it has no terminal attached.

```bash
bun run build
```

`bun run build` writes the release app to `build/`.

Other scripts:

- `bun run generate` regenerates `src/mygo.ts`, the typed client for the Go methods, after you change them.
- `bun run typecheck` and `go vet ./...` check the code.
- `go test ./...` runs the Go tests.

### Where things are

| Path | What |
| --- | --- |
| `main.go` | Window, menus and app startup |
| `library.go` | Folders, scanning and watching, indexing, ratings and tags, trash, and the `eagle://` scheme that serves thumbnails and previews |
| `imaging.go` | Dimensions and orientation, thumbnails and previews, palettes and colour families |
| `src/main.ts` | The whole interface: sidebar, grid, filters, inspector and viewer |
| `src/style.css` | Styles, light and dark |
| `src/icons.ts` | Interface icons |
| `src/mygo.ts` | Generated client for the Go methods (don't edit) |
| `resources/icon.png` | App icon |
| `design/` | Icon source: `make-icon.py` writes `icon.svg` |

The library (folders, ratings, favourites and tags) is saved in `~/Library/Application Support/eagle/library.json`. Thumbnails, previews and the index are kept in `~/Library/Caches/eagle/`. Ratings and tags live in Eagle's library, not in your image files. Development builds use `eagle Dev` in place of `eagle` for both folders.
