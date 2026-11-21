// Eagle's page: the library sidebar, the photo grid, filters, the inspector
// and the viewer. Go owns the library (src/mygo.ts is generated from it);
// this page keeps a copy of it and renders what the user asks to see.
import { isMyGo, onFileDrop } from "mygo-runtime";
import { Library, events, type Folder, type Image, type Kind } from "./mygo";
import { icons } from "./icons";

if (isMyGo()) document.documentElement.classList.add("mygo");

// ---------------------------------------------------------------- helpers

const $ = <T extends HTMLElement = HTMLElement>(sel: string) => document.querySelector<T>(sel)!;
const ESC: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
const esc = (s: string) => s.replace(/[&<>"']/g, (c) => ESC[c]!);
const DAY = 864e5;
const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

function fmtBytes(n: number): string {
  if (n < 1000) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let i = -1;
  do {
    n /= 1000;
    i++;
  } while (n >= 1000 && i < units.length - 1);
  return `${n >= 100 ? Math.round(n) : n.toFixed(1)} ${units[i]}`;
}
const fmtInt = (n: number) => n.toLocaleString();
const plural = (n: number, one: string, many = one + "s") => `${fmtInt(n)} ${n === 1 ? one : many}`;
const startOfDay = (t = Date.now()) => new Date(new Date(t).toDateString()).getTime();
const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const dateFmt = new Intl.DateTimeFormat(undefined, { weekday: "short", day: "numeric", month: "short", year: "numeric" });
const monthFmt = new Intl.DateTimeFormat(undefined, { month: "long" });
const monthYearFmt = new Intl.DateTimeFormat(undefined, { month: "long", year: "numeric" });

function fmtDate(ms: number): string {
  const today = startOfDay();
  if (ms >= today) return `Today, ${timeFmt.format(ms)}`;
  if (ms >= today - DAY) return `Yesterday, ${timeFmt.format(ms)}`;
  return `${dateFmt.format(ms)}, ${timeFmt.format(ms)}`;
}

function groupLabel(ms: number): string {
  const today = startOfDay();
  if (ms >= today) return "Today";
  if (ms >= today - DAY) return "Yesterday";
  if (ms >= today - 6 * DAY) return "This Week";
  const d = new Date(ms);
  const now = new Date();
  if (d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth()) return `Earlier in ${monthFmt.format(d)}`;
  return monthYearFmt.format(d);
}

const errorMessage = (e: unknown) => (e instanceof Error ? e.message : String(e)).replace(/^mygo: /, "");
const fail = (e: unknown) => toast(errorMessage(e));

function hash(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (Math.imul(h, 31) + s.charCodeAt(i)) | 0;
  return Math.abs(h);
}

// ---------------------------------------------------------------- vocabulary

type Shape = "landscape" | "portrait" | "square" | "panorama";
type DateRange = "any" | "today" | "week" | "month" | "year";
type SortKey = "modified" | "name" | "size" | "dimensions";
type Layout = "justified" | "grid";
type Source =
  | { kind: "all" | "recent" | "favorites" | "untagged" }
  | { kind: "folder"; id: string; dir: string }
  | { kind: "tag"; tag: string };

const COLORS = [
  { name: "red", hex: "#E5484D" },
  { name: "orange", hex: "#F76B15" },
  { name: "yellow", hex: "#F5C443" },
  { name: "green", hex: "#46A758" },
  { name: "teal", hex: "#12A594" },
  { name: "blue", hex: "#3E7BFA" },
  { name: "purple", hex: "#8E4EC6" },
  { name: "pink", hex: "#E93D82" },
  { name: "brown", hex: "#8F6A4A" },
  { name: "black", hex: "#1C1C1E" },
  { name: "gray", hex: "#8E8E93" },
  { name: "white", hex: "#F5F5F7" },
];
const colorHex = (name: string) => COLORS.find((c) => c.name === name)?.hex ?? "#8E8E93";

const KINDS: Kind[] = ["JPG", "HEIC", "PNG", "WEBP", "GIF", "RAW", "TIFF", "AVIF", "BMP"];
const KIND_LABEL: Record<Kind, string> = {
  JPG: "JPEG image", PNG: "PNG image", HEIC: "HEIC image", WEBP: "WebP image", GIF: "GIF image",
  TIFF: "TIFF image", BMP: "BMP image", AVIF: "AVIF image", RAW: "Camera RAW",
};
const KIND_COLOR: Record<Kind, string> = {
  JPG: "#3E7BFA", HEIC: "#12A594", PNG: "#F5C443", WEBP: "#8E4EC6", GIF: "#E93D82",
  RAW: "#F76B15", TIFF: "#8F6A4A", AVIF: "#46A758", BMP: "#8E8E93",
};
const TAG_COLORS = ["#46A758", "#F76B15", "#3E7BFA", "#8E4EC6", "#E93D82", "#12A594", "#F5C443", "#E5484D"];
const tagColor = (t: string) => TAG_COLORS[hash(t) % TAG_COLORS.length]!;

const SHAPES: { id: Shape; label: string; icon: string }[] = [
  { id: "landscape", label: "Landscape", icon: icons.landscape },
  { id: "portrait", label: "Portrait", icon: icons.portrait },
  { id: "square", label: "Square", icon: icons.square },
  { id: "panorama", label: "Panorama", icon: icons.panorama },
];
function shapeOf(img: Image): Shape | null {
  if (!img.width || !img.height) return null;
  const r = img.width / img.height;
  if (r >= 2) return "panorama";
  if (r > 1.1) return "landscape";
  if (r < 0.9) return "portrait";
  return "square";
}

const RESOLUTIONS = [
  { min: 0, label: "Any size", chip: "" },
  { min: 1920, label: "HD and up", chip: "HD and up" },
  { min: 3840, label: "4K and up", chip: "4K and up" },
  { min: 5120, label: "5K and up", chip: "5K and up" },
];
const DATES: { id: DateRange; label: string }[] = [
  { id: "any", label: "Any time" },
  { id: "today", label: "Today" },
  { id: "week", label: "Past 7 days" },
  { id: "month", label: "Past 30 days" },
  { id: "year", label: "This year" },
];
const SORTS: { id: SortKey; label: string }[] = [
  { id: "modified", label: "Date Modified" },
  { id: "name", label: "Name" },
  { id: "size", label: "File Size" },
  { id: "dimensions", label: "Dimensions" },
];
const RECENT_DAYS = 30;

const thumbURL = (img: Image) => `eagle://localhost/thumb/${img.id}?v=${img.version}`;
const fullURL = (img: Image) => `eagle://localhost/full/${img.id}?v=${img.version}`;
const dims = (img: Image) => (img.width ? `${fmtInt(img.width)} × ${fmtInt(img.height)}` : "—");
const aspect = (img: Image) => (img.width && img.height ? clamp(img.width / img.height, 0.33, 4) : 1.5);
const stars = (n: number) => "★".repeat(n) + "☆".repeat(5 - n);

// ---------------------------------------------------------------- state

interface Filters {
  query: string;
  colors: Set<string>;
  kinds: Set<Kind>;
  shapes: Set<Shape>;
  minRes: number;
  minRating: number;
  date: DateRange;
}
const emptyFilters = (): Filters => ({
  query: "", colors: new Set(), kinds: new Set(), shapes: new Set(), minRes: 0, minRating: 0, date: "any",
});

interface Prefs { layout: Layout; zoom: number; inspector: boolean; filterbar: boolean; sortKey: SortKey; sortDir: 1 | -1 }
const PREFS_KEY = "eagle.prefs.v1";
function loadPrefs(): Prefs {
  const base: Prefs = { layout: "justified", zoom: 220, inspector: true, filterbar: true, sortKey: "modified", sortDir: -1 };
  try {
    return { ...base, ...JSON.parse(localStorage.getItem(PREFS_KEY) ?? "{}") };
  } catch {
    return base;
  }
}

const S = {
  loaded: false,
  folders: [] as Folder[],
  images: new Map<string, Image>(),
  source: { kind: "all" } as Source,
  f: emptyFilters(),
  ...loadPrefs(),
  selection: new Set<string>(),
  anchor: null as string | null,
  focus: null as string | null,
  expanded: new Set<string>(),
};

function savePrefs() {
  const p: Prefs = { layout: S.layout, zoom: S.zoom, inspector: S.inspector, filterbar: S.filterbar, sortKey: S.sortKey, sortDir: S.sortDir };
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(p));
  } catch {
    // Preferences are a convenience; the page works without them.
  }
}

// ---------------------------------------------------------------- the list

function inSource(img: Image, src: Source = S.source): boolean {
  switch (src.kind) {
    case "all":
      return true;
    case "recent":
      return img.modified >= Date.now() - RECENT_DAYS * DAY;
    case "favorites":
      return img.favorite;
    case "untagged":
      return img.tags.length === 0;
    case "tag":
      return img.tags.includes(src.tag);
    case "folder":
      return img.folder === src.id && (!src.dir || img.dir === src.dir || img.dir.startsWith(src.dir + "/"));
  }
}

function folderName(id: string) {
  return S.folders.find((f) => f.id === id)?.name ?? "";
}

function matches(img: Image, except?: keyof Filters): boolean {
  const f = S.f;
  if (except !== "query" && f.query) {
    const hay = `${img.name} ${img.tags.join(" ")} ${img.dir} ${folderName(img.folder)}`.toLowerCase();
    for (const word of f.query.toLowerCase().split(/\s+/)) if (word && !hay.includes(word)) return false;
  }
  if (except !== "colors" && f.colors.size && !img.colors.some((c) => f.colors.has(c))) return false;
  if (except !== "kinds" && f.kinds.size && !f.kinds.has(img.kind)) return false;
  if (except !== "shapes" && f.shapes.size) {
    const s = shapeOf(img);
    if (!s || !f.shapes.has(s)) return false;
  }
  if (except !== "minRes" && f.minRes && Math.max(img.width, img.height) < f.minRes) return false;
  if (except !== "minRating" && f.minRating && img.rating < f.minRating) return false;
  if (except !== "date" && f.date !== "any") {
    const now = Date.now();
    const since =
      f.date === "today" ? startOfDay() :
      f.date === "week" ? now - 7 * DAY :
      f.date === "month" ? now - 30 * DAY :
      new Date(new Date().getFullYear(), 0, 1).getTime();
    if (img.modified < since) return false;
  }
  return true;
}

const hasFilters = () => {
  const f = S.f;
  return f.colors.size + f.kinds.size + f.shapes.size > 0 || f.minRes > 0 || f.minRating > 0 || f.date !== "any";
};

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
function compare(a: Image, b: Image): number {
  let d = 0;
  switch (S.sortKey) {
    case "modified": d = a.modified - b.modified; break;
    case "name": d = collator.compare(a.name, b.name); break;
    case "size": d = a.size - b.size; break;
    case "dimensions": d = a.width * a.height - b.width * b.height; break;
  }
  return d * S.sortDir || collator.compare(a.name, b.name);
}

let listCache: Image[] | null = null;
function list(): Image[] {
  if (!listCache) {
    listCache = [];
    for (const img of S.images.values()) if (inSource(img) && matches(img)) listCache.push(img);
    listCache.sort(compare);
  }
  return listCache;
}

function sourceImages(except?: keyof Filters): Image[] {
  const out: Image[] = [];
  for (const img of S.images.values()) if (inSource(img) && (except === undefined || matches(img, except))) out.push(img);
  return out;
}

/** Recomputes the list after the library or the filters changed. */
function refresh() {
  const before = listCache;
  listCache = null;
  const after = list();
  if (!before || before.length !== after.length || before.some((x, i) => x !== after[i])) layoutDirty = true;
  // Only what shows can be selected, so nothing hidden by a filter is
  // trashed or tagged by mistake.
  if (S.selection.size || S.focus) {
    const shown = new Set(after.map((i) => i.id));
    for (const id of S.selection) if (!shown.has(id)) S.selection.delete(id);
    if (S.focus && !shown.has(S.focus)) S.focus = null;
    if (S.anchor && !shown.has(S.anchor)) S.anchor = null;
  }
  schedule("all");
}

// ---------------------------------------------------------------- render loop

type Part = "all" | "chrome" | "sidebar" | "toolbar" | "filterbar" | "grid" | "selection" | "inspector" | "status";
const dirty = new Set<Part>();
let frame = 0;
function schedule(...parts: Part[]) {
  for (const p of parts) dirty.add(p);
  if (!frame) frame = requestAnimationFrame(flush);
}
function flush() {
  frame = 0;
  const d = new Set(dirty);
  dirty.clear();
  const all = d.has("all");
  if (all || d.has("chrome")) renderChrome();
  if (all || d.has("sidebar")) renderSidebar();
  if (all || d.has("toolbar")) renderToolbar();
  if (all || d.has("filterbar")) renderFilterbar();
  if (all || d.has("grid")) renderGrid();
  if (all || d.has("grid") || d.has("selection")) renderSelection();
  if (all || d.has("inspector") || d.has("selection")) renderInspector();
  if (all || d.has("status") || d.has("selection")) renderStatus();
}

const win = $("#window");
const content = $("#content");
const canvas = $("#canvas");

const isWelcome = () => S.loaded && S.folders.length === 0;

function renderChrome() {
  win.classList.toggle("welcome", isWelcome());
  win.classList.toggle("no-inspector", !S.inspector);
  $("#filterbar").classList.toggle("collapsed", !S.filterbar);
}

// ---------------------------------------------------------------- sidebar

function renderSidebar() {
  const imgs = [...S.images.values()];
  const recentSince = Date.now() - RECENT_DAYS * DAY;
  const n = {
    all: imgs.length,
    recent: imgs.filter((i) => i.modified >= recentSince).length,
    favorites: imgs.filter((i) => i.favorite).length,
    untagged: imgs.filter((i) => i.tags.length === 0).length,
  };
  const subdirs = new Map<string, Map<string, number>>();
  const tags = new Map<string, number>();
  const byKind = new Map<Kind, number>();
  let bytes = 0;
  for (const img of imgs) {
    if (img.dir) {
      const top = img.dir.split("/")[0]!;
      let m = subdirs.get(img.folder);
      if (!m) subdirs.set(img.folder, (m = new Map()));
      m.set(top, (m.get(top) ?? 0) + 1);
    }
    for (const t of img.tags) tags.set(t, (tags.get(t) ?? 0) + 1);
    byKind.set(img.kind, (byKind.get(img.kind) ?? 0) + img.size);
    bytes += img.size;
  }
  const src = S.source;
  const active = (k: string) => src.kind === k;
  const item = (key: string, icon: string, label: string, count: number) =>
    `<button type="button" class="side-item${active(key) ? " active" : ""}" data-src="${key}">${icon}<span class="label">${label}</span><span class="count">${fmtInt(count)}</span></button>`;

  let folders = "";
  for (const f of S.folders) {
    const subs = [...(subdirs.get(f.id) ?? new Map<string, number>())].sort((a, b) => collator.compare(a[0], b[0]));
    const open = S.expanded.has(f.id);
    const isActive = src.kind === "folder" && src.id === f.id && !src.dir;
    folders += `<button type="button" class="side-item${isActive ? " active" : ""}${f.missing ? " missing" : ""}" data-folder="${f.id}" title="${esc(f.path)}" style="padding-left:4px">
      <span class="disclose${open ? " open" : ""}${subs.length ? "" : " none"}" data-toggle="${f.id}">${icons.chevron}</span>
      ${f.missing ? icons.folderAlert : icons.folder}<span class="label">${esc(f.name)}</span><span class="count">${fmtInt(f.count)}</span></button>`;
    if (f.indexing && f.indexing.total > 0) {
      const pct = Math.round((f.indexing.done / f.indexing.total) * 100);
      folders += `<div class="indexing"><span class="bar"><i style="width:${pct}%"></i></span><span class="num">Indexing ${fmtInt(f.indexing.done)} / ${fmtInt(f.indexing.total)}</span></div>`;
    }
    if (open) {
      for (const [dir, count] of subs) {
        const on = src.kind === "folder" && src.id === f.id && src.dir === dir;
        folders += `<button type="button" class="side-item child${on ? " active" : ""}" data-folder="${f.id}" data-dir="${esc(dir)}">${icons.folder}<span class="label">${esc(dir)}</span><span class="count">${fmtInt(count)}</span></button>`;
      }
    }
  }
  if (!S.folders.length) {
    folders = `<button type="button" class="add-folder" data-act="add-folder">${icons.folderPlus}<span>Add Folder…</span></button>`;
  }

  const topTags = [...tags].sort((a, b) => b[1] - a[1] || collator.compare(a[0], b[0])).slice(0, 12);
  const tagHTML = topTags
    .map(([t, c]) => {
      const on = src.kind === "tag" && src.tag === t;
      return `<button type="button" class="side-item${on ? " active" : ""}" data-tag="${esc(t)}"><span class="dot" style="background:${tagColor(t)}"></span><span class="label">${esc(t)}</span><span class="count">${fmtInt(c)}</span></button>`;
    })
    .join("");

  const storage = KINDS.filter((k) => byKind.get(k))
    .map((k) => `<i style="flex:${byKind.get(k)};background:${KIND_COLOR[k]}" title="${k} · ${fmtBytes(byKind.get(k)!)}"></i>`)
    .join("");

  $("#sidebar").innerHTML = `
    <div class="side-scroll">
      <div class="side-section">
        <div class="side-heading">Library</div>
        ${item("all", icons.photos, "All Images", n.all)}
        ${item("recent", icons.clock, "Recent", n.recent)}
        ${item("favorites", icons.heart, "Favorites", n.favorites)}
        ${item("untagged", icons.tag, "Untagged", n.untagged)}
      </div>
      <div class="side-section">
        <div class="side-heading"><span>Folders</span><button type="button" data-act="add-folder" aria-label="Add folder" title="Add Folder… (⌘O)">${icons.plus}</button></div>
        ${folders}
      </div>
      ${topTags.length ? `<div class="side-section"><div class="side-heading">Tags</div>${tagHTML}</div>` : ""}
    </div>
    ${imgs.length ? `<div class="side-foot"><div class="storage">${storage}</div><span class="num">${plural(imgs.length, "image")} · ${fmtBytes(bytes)}</span></div>` : ""}`;
}

$("#sidebar").addEventListener("click", (e) => {
  const t = e.target as HTMLElement;
  const toggle = t.closest<HTMLElement>("[data-toggle]");
  if (toggle && !toggle.classList.contains("none")) {
    const id = toggle.dataset.toggle!;
    if (S.expanded.has(id)) S.expanded.delete(id);
    else S.expanded.add(id);
    schedule("sidebar");
    return;
  }
  if (t.closest("[data-act=add-folder]")) return void addFolder();
  const el = t.closest<HTMLElement>("[data-src],[data-folder],[data-tag]");
  if (!el) return;
  if (el.dataset.src) setSource({ kind: el.dataset.src as "all" });
  else if (el.dataset.folder) setSource({ kind: "folder", id: el.dataset.folder, dir: el.dataset.dir ?? "" });
  else if (el.dataset.tag) setSource({ kind: "tag", tag: el.dataset.tag });
});

$("#sidebar").addEventListener("contextmenu", (e) => {
  const el = (e.target as HTMLElement).closest<HTMLElement>("[data-folder]");
  if (!el || el.dataset.dir) return;
  e.preventDefault();
  const id = el.dataset.folder!;
  openMenu({ x: e.clientX, y: e.clientY }, [
    { label: "Show in Finder", run: () => Library.openFolder(id).catch(fail) },
    { label: "Look for Changes", run: () => Library.rescan(id).catch(fail) },
    "-",
    {
      label: "Remove from Library",
      danger: true,
      run: () => Library.removeFolder(id).then(() => toast(`Removed ${folderName(id) || "the folder"}. Its files are untouched.`)).catch(fail),
    },
  ]);
});

function setSource(src: Source) {
  S.source = src;
  S.selection.clear();
  S.focus = S.anchor = null;
  content.scrollTop = 0;
  refresh();
  layoutDirty = true;
}

function sourceTitle(): string {
  const src = S.source;
  switch (src.kind) {
    case "all": return "All Images";
    case "recent": return "Recent";
    case "favorites": return "Favorites";
    case "untagged": return "Untagged";
    case "tag": return src.tag;
    case "folder": {
      const name = folderName(src.id) || "Folder";
      return src.dir ? `${name} › ${src.dir.replaceAll("/", " › ")}` : name;
    }
  }
}

// ---------------------------------------------------------------- toolbar

$("#layout-justified").innerHTML = icons.justified;
$("#layout-grid").innerHTML = icons.grid;
$("#zoom-small").innerHTML = icons.small;
$("#zoom-large").innerHTML = icons.large;
$("#search-icon").innerHTML = icons.search;
$("#search-clear").innerHTML = icons.closeSmall;
$("#toggle-inspector").innerHTML = icons.sidebarRight;
$("#dropzone-icon").innerHTML = icons.drop;

const search = $<HTMLInputElement>("#search");
const zoomInput = $<HTMLInputElement>("#zoom");

function renderToolbar() {
  const items = list();
  const total = sourceImages().length;
  $("#title").textContent = isWelcome() ? "Eagle" : sourceTitle();
  const filtered = hasFilters() || S.f.query;
  let sub = filtered
    ? `${fmtInt(items.length)} of ${plural(total, "image")} match`
    : `${plural(items.length, "image")} · ${fmtBytes(items.reduce((a, i) => a + i.size, 0))}`;
  if (S.source.kind === "folder") {
    const f = S.folders.find((x) => S.source.kind === "folder" && x.id === S.source.id);
    if (f?.indexing && f.indexing.total) sub += ` · Indexing ${fmtInt(f.indexing.done)} of ${fmtInt(f.indexing.total)}`;
    if (f?.missing) sub = "Folder not found";
  }
  $("#subtitle").textContent = isWelcome() ? "" : sub;
  $("#layout-justified").classList.toggle("on", S.layout === "justified");
  $("#layout-grid").classList.toggle("on", S.layout === "grid");
  zoomInput.value = String(S.zoom);
  $("#sort").innerHTML = `${icons.sort}<span>${SORTS.find((s) => s.id === S.sortKey)!.label}</span>`;
  if (document.activeElement !== search) search.value = S.f.query;
  search.parentElement!.classList.toggle("filled", !!S.f.query);
  $("#search-clear").hidden = !S.f.query;
  $("#toggle-inspector").classList.toggle("on", S.inspector);
}

$("#layout-justified").addEventListener("click", () => setLayout("justified"));
$("#layout-grid").addEventListener("click", () => setLayout("grid"));
zoomInput.addEventListener("input", () => setZoom(Number(zoomInput.value)));
$("#toggle-inspector").addEventListener("click", () => toggleInspector());
search.addEventListener("input", () => {
  S.f.query = search.value.trim();
  refresh();
});
search.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    if (search.value) {
      search.value = "";
      S.f.query = "";
      refresh();
    } else search.blur();
    e.stopPropagation();
  } else if (e.key === "Enter" || e.key === "ArrowDown") {
    search.blur();
    const first = list()[0];
    if (first && !S.selection.size) selectOnly(first.id);
    content.focus();
  }
});
$("#search-clear").addEventListener("click", () => {
  search.value = "";
  S.f.query = "";
  refresh();
  search.focus();
});
$("#sort").addEventListener("click", (e) => openSortMenu(e.currentTarget as HTMLElement));

function setLayout(layout: Layout) {
  if (S.layout === layout) return;
  S.layout = layout;
  layoutDirty = true;
  savePrefs();
  schedule("toolbar", "grid");
}
function setZoom(zoom: number) {
  S.zoom = clamp(Math.round(zoom / 10) * 10, 100, 380);
  layoutDirty = true;
  savePrefs();
  schedule("toolbar", "grid");
}
function toggleInspector(on = !S.inspector) {
  S.inspector = on;
  savePrefs();
  schedule("chrome", "toolbar", "inspector");
}

// ---------------------------------------------------------------- filter bar

function chipLabel(label: string, value: string) {
  return value ? `${label}: <strong>${esc(value)}</strong>` : label;
}

function renderFilterbar() {
  const f = S.f;
  const colorCounts = new Map<string, number>();
  for (const img of sourceImages("colors")) for (const c of img.colors) colorCounts.set(c, (colorCounts.get(c) ?? 0) + 1);
  const swatches = COLORS.map((c) => {
    const n = colorCounts.get(c.name) ?? 0;
    const on = f.colors.has(c.name);
    const cap = c.name[0]!.toUpperCase() + c.name.slice(1);
    return `<button type="button" class="swatch${on ? " on" : ""}${n === 0 && !on ? " none" : ""}" data-color="${c.name}" style="background:${c.hex}" aria-pressed="${on}" aria-label="${cap}" title="${cap} · ${plural(n, "image")}"></button>`;
  }).join("");

  const list2 = (xs: string[]) => (xs.length <= 2 ? xs.join(", ") : `${xs.length} selected`);
  const kinds = KINDS.filter((k) => f.kinds.has(k));
  const shapes = SHAPES.filter((s) => f.shapes.has(s.id)).map((s) => s.label);
  const res = RESOLUTIONS.find((r) => r.min === f.minRes)!.chip;
  const rating = f.minRating ? (f.minRating === 5 ? "★★★★★" : `${"★".repeat(f.minRating)} and up`) : "";
  const date = f.date === "any" ? "" : DATES.find((d) => d.id === f.date)!.label;
  const chip = (id: string, label: string, value: string) =>
    `<button type="button" class="chip${value ? " on" : ""}" data-chip="${id}" aria-haspopup="true">${chipLabel(label, value)}${icons.chevronDown}</button>`;

  $("#filterbar").innerHTML = `
    <div class="swatches${f.colors.size ? " some" : ""}" role="group" aria-label="Filter by colour">${swatches}</div>
    ${chip("kinds", "Type", list2(kinds))}
    ${chip("shapes", "Shape", list2(shapes))}
    ${chip("res", "Resolution", res)}
    ${chip("rating", "Rating", rating)}
    ${chip("date", "Date", date)}
    <span class="spacer"></span>
    ${hasFilters() ? `<button type="button" class="link" data-act="clear-filters">Clear filters</button>` : ""}`;
}

$("#filterbar").addEventListener("click", (e) => {
  const t = e.target as HTMLElement;
  const sw = t.closest<HTMLElement>("[data-color]");
  if (sw) {
    toggleColor(sw.dataset.color!, e.metaKey || e.shiftKey);
    return;
  }
  const chip = t.closest<HTMLElement>("[data-chip]");
  if (chip) return openFilterPopover(chip, chip.dataset.chip!);
  if (t.closest("[data-act=clear-filters]")) clearFilters();
});

/** Picks a color to filter by; with ⌘ or ⇧ it adds to the colors picked. */
function toggleColor(name: string, add = false) {
  const c = S.f.colors;
  if (c.has(name)) c.delete(name);
  else {
    if (!add) c.clear();
    c.add(name);
  }
  refresh();
}

function clearFilters() {
  const q = S.f.query;
  S.f = emptyFilters();
  S.f.query = q;
  refresh();
}

// ---------------------------------------------------------------- popovers and menus

const popover = $("#popover");
let pop: { anchor: HTMLElement | null; render: () => string; act: (el: HTMLElement) => boolean | void } | null = null;

function showPopover(at: HTMLElement | { x: number; y: number }, render: () => string, act: (el: HTMLElement) => boolean | void) {
  pop = { anchor: at instanceof HTMLElement ? at : null, render, act };
  popover.innerHTML = render();
  popover.hidden = false;
  const r = at instanceof HTMLElement ? at.getBoundingClientRect() : { left: at.x, bottom: at.y, top: at.y };
  const pw = popover.offsetWidth;
  const ph = popover.offsetHeight;
  let x = r.left;
  let y = r.bottom + (at instanceof HTMLElement ? 6 : 2);
  if (x + pw > innerWidth - 8) x = innerWidth - pw - 8;
  if (y + ph > innerHeight - 8) y = Math.max(8, (at instanceof HTMLElement ? r.top - 6 : r.top) - ph);
  popover.style.left = `${Math.max(8, x)}px`;
  popover.style.top = `${y}px`;
}

function closePopover() {
  pop = null;
  popover.hidden = true;
}

popover.addEventListener("click", (e) => {
  const el = (e.target as HTMLElement).closest<HTMLElement>("[data-act]");
  if (!el || !pop || el.hasAttribute("disabled")) return;
  const keepOpen = pop.act(el);
  if (keepOpen && pop) popover.innerHTML = pop.render();
  else closePopover();
});
document.addEventListener("mousedown", (e) => {
  if (!pop) return;
  const t = e.target as Node;
  if (popover.contains(t) || (pop.anchor && pop.anchor.contains(t))) return;
  closePopover();
}, true);
window.addEventListener("blur", closePopover);
window.addEventListener("resize", closePopover);

const chk = (on: boolean) => `<span class="chk">${on ? icons.check : ""}</span>`;
const popItem = (act: string, value: string, on: boolean, label: string, n?: number | string, extra = "") =>
  `<button type="button" class="pop-item" data-act="${act}" data-v="${esc(value)}"${extra}>${chk(on)}<span class="lbl">${label}</span>${n !== undefined ? `<span class="n">${typeof n === "number" ? fmtInt(n) : n}</span>` : ""}</button>`;

function openFilterPopover(anchor: HTMLElement, which: string) {
  if (pop && pop.anchor === anchor) return closePopover();
  const foot = () => `<div class="pop-sep"></div><div class="pop-foot"><span>Showing ${plural(list().length, "image")}</span><button type="button" class="link" data-act="reset">Reset</button></div>`;
  const f = () => S.f;
  const render = (): string => {
    switch (which) {
      case "kinds": {
        const counts = new Map<Kind, number>();
        for (const img of sourceImages("kinds")) counts.set(img.kind, (counts.get(img.kind) ?? 0) + 1);
        const kinds = KINDS.filter((k) => counts.get(k) || f().kinds.has(k));
        return `<div class="pop-title">File type</div>${kinds.map((k) => popItem("kind", k, f().kinds.has(k), k, counts.get(k) ?? 0)).join("") || `<div class="pop-foot">No images</div>`}${foot()}`;
      }
      case "shapes": {
        const counts = new Map<Shape, number>();
        for (const img of sourceImages("shapes")) {
          const s = shapeOf(img);
          if (s) counts.set(s, (counts.get(s) ?? 0) + 1);
        }
        return `<div class="pop-title">Shape</div>${SHAPES.map((s) => popItem("shape", s.id, f().shapes.has(s.id), `<span style="display:flex;align-items:center;gap:8px">${s.icon}${s.label}</span>`, counts.get(s.id) ?? 0)).join("")}${foot()}`;
      }
      case "res": {
        const imgs = sourceImages("minRes");
        return `<div class="pop-title">Long edge</div>${RESOLUTIONS.map((r) => popItem("res", String(r.min), f().minRes === r.min, r.label, imgs.filter((i) => Math.max(i.width, i.height) >= r.min).length)).join("")}${foot()}`;
      }
      case "rating": {
        const imgs = sourceImages("minRating");
        const rows = [0, 1, 2, 3, 4, 5].map((n) =>
          popItem("rating", String(n), f().minRating === n, n === 0 ? "Any rating" : `<span style="color:var(--star);letter-spacing:1px">${"★".repeat(n)}</span>${n < 5 ? " and up" : ""}`, imgs.filter((i) => i.rating >= n).length));
        return `<div class="pop-title">Rating</div>${rows.join("")}${foot()}`;
      }
      default: {
        return `<div class="pop-title">Modified</div>${DATES.map((d) => popItem("date", d.id, f().date === d.id, d.label)).join("")}${foot()}`;
      }
    }
  };
  showPopover(anchor, render, (el) => {
    const v = el.dataset.v!;
    const fl = S.f;
    switch (el.dataset.act) {
      case "kind": fl.kinds.has(v as Kind) ? fl.kinds.delete(v as Kind) : fl.kinds.add(v as Kind); break;
      case "shape": fl.shapes.has(v as Shape) ? fl.shapes.delete(v as Shape) : fl.shapes.add(v as Shape); break;
      case "res": fl.minRes = Number(v); break;
      case "rating": fl.minRating = Number(v); break;
      case "date": fl.date = v as DateRange; break;
      case "reset":
        if (which === "kinds") fl.kinds.clear();
        else if (which === "shapes") fl.shapes.clear();
        else if (which === "res") fl.minRes = 0;
        else if (which === "rating") fl.minRating = 0;
        else fl.date = "any";
        break;
    }
    refresh();
    // Lists of choices stay open; single choices close.
    return which === "kinds" || which === "shapes";
  });
}

function openSortMenu(anchor: HTMLElement) {
  if (pop && pop.anchor === anchor) return closePopover();
  showPopover(anchor, () =>
    `<div class="pop-title">Sort by</div>${SORTS.map((s) => popItem("key", s.id, S.sortKey === s.id, s.label)).join("")}
     <div class="pop-sep"></div>
     ${popItem("dir", "-1", S.sortDir === -1, S.sortKey === "name" ? "Z to A" : S.sortKey === "modified" ? "Newest first" : "Largest first")}
     ${popItem("dir", "1", S.sortDir === 1, S.sortKey === "name" ? "A to Z" : S.sortKey === "modified" ? "Oldest first" : "Smallest first")}`,
    (el) => {
      if (el.dataset.act === "key") {
        const key = el.dataset.v as SortKey;
        if (key !== S.sortKey) S.sortDir = key === "name" ? 1 : -1;
        S.sortKey = key;
      } else S.sortDir = Number(el.dataset.v) as 1 | -1;
      savePrefs();
      listCache = null;
      refresh();
      layoutDirty = true;
      return true;
    });
}

type MenuItem = "-" | { label: string; kbd?: string; danger?: boolean; disabled?: boolean; run: () => void };
function openMenu(at: { x: number; y: number }, items: MenuItem[]) {
  const runs: (() => void)[] = [];
  showPopover(at, () =>
    items.map((it) => {
      if (it === "-") return `<div class="pop-sep"></div>`;
      runs.push(it.run);
      return `<button type="button" class="pop-item${it.danger ? " danger" : ""}" data-act="menu" data-v="${runs.length - 1}"${it.disabled ? " disabled" : ""}><span class="lbl">${esc(it.label)}</span>${it.kbd ? `<kbd>${it.kbd}</kbd>` : ""}</button>`;
    }).join(""),
    (el) => {
      runs[Number(el.dataset.v)]?.();
    });
}

// ---------------------------------------------------------------- grid layout

interface Placed { img: Image; x: number; w: number; h: number }
interface Block { top: number; height: number; head?: { label: string; count: number }; items?: Placed[] }

let blocks: Block[] = [];
let layoutDirty = true;
let layoutWidth = 0;
const where = new Map<string, { block: number; x: number; w: number; top: number; h: number }>();
const mountedBlocks = new Map<number, HTMLElement>();
const tiles = new Map<string, HTMLElement>(); // mounted tiles by image ID

const GAP = 6;
const HEAD = 46;

function justify(items: Image[], width: number, target: number): Placed[][] {
  const rows: Placed[][] = [];
  let row: Image[] = [];
  let sum = 0;
  const place = (row: Image[], h: number, full: boolean) => {
    let x = 0;
    const out = row.map((img, i) => {
      const w = i === row.length - 1 && full ? width - x : Math.round(aspect(img) * h);
      const p = { img, x, w, h: Math.round(h) };
      x += w + GAP;
      return p;
    });
    rows.push(out);
  };
  for (const img of items) {
    row.push(img);
    sum += aspect(img);
    if (sum * target + GAP * (row.length - 1) >= width) {
      place(row, (width - GAP * (row.length - 1)) / sum, true);
      row = [];
      sum = 0;
    }
  }
  if (row.length) place(row, Math.min(target, (width - GAP * (row.length - 1)) / sum), false);
  return rows;
}

function computeLayout() {
  const width = Math.max(200, content.clientWidth - 40);
  layoutWidth = content.clientWidth;
  blocks = [];
  where.clear();
  const items = list();
  const groups: { label: string; items: Image[] }[] = [];
  if (S.sortKey === "modified") {
    for (const img of items) {
      const label = groupLabel(img.modified);
      const last = groups[groups.length - 1];
      if (last && last.label === label) last.items.push(img);
      else groups.push({ label, items: [img] });
    }
  } else groups.push({ label: "", items });

  let y = groups[0]?.label ? 0 : 16;
  for (const g of groups) {
    if (g.label) {
      blocks.push({ top: y, height: HEAD, head: { label: g.label, count: g.items.length } });
      y += HEAD;
    }
    let rows: Placed[][];
    let rowGap = GAP;
    if (S.layout === "justified") rows = justify(g.items, width, S.zoom);
    else {
      const min = Math.round(S.zoom * 0.72);
      const colGap = 14;
      const cols = Math.max(1, Math.floor((width + colGap) / (min + colGap)));
      const cw = (width - colGap * (cols - 1)) / cols;
      rows = [];
      for (let i = 0; i < g.items.length; i += cols)
        rows.push(g.items.slice(i, i + cols).map((img, c) => ({ img, x: Math.round(c * (cw + colGap)), w: Math.floor(cw), h: Math.floor(cw) + 40 })));
      rowGap = 18;
    }
    for (const row of rows) {
      const h = row[0]!.h;
      const index = blocks.length;
      blocks.push({ top: y, height: h, items: row });
      for (const p of row) where.set(p.img.id, { block: index, x: p.x, w: p.w, top: y, h });
      y += h + rowGap;
    }
    y += 4;
  }
  canvas.style.height = `${y + 24}px`;
}

function tileFor(p: Placed): HTMLElement {
  const img = p.img;
  const grid = S.layout === "grid";
  let el = tiles.get(img.id);
  if (!el || el.dataset.layout !== S.layout || el.dataset.v !== img.version) {
    el = document.createElement("div");
    el.dataset.id = img.id;
    el.dataset.layout = S.layout;
    el.dataset.v = img.version;
    el.className = grid ? "card" : "tile";
    el.innerHTML = grid
      ? `<div class="frame"><img alt="" decoding="async" src="${thumbURL(img)}"><span class="deco"></span></div><div class="caption"></div>`
      : `<img alt="" decoding="async" src="${thumbURL(img)}"><span class="deco"></span>`;
  }
  el.style.left = `${p.x}px`;
  el.style.width = `${p.w}px`;
  el.style.height = `${p.h}px`;
  decorate(el, img);
  return el;
}

/** Updates what a tile shows besides its picture: badges, the favorite
 * heart, and its caption. */
function decorate(el: HTMLElement, img: Image) {
  const sig = `${img.favorite}|${img.width}|${img.name}|${img.palette[0] ?? ""}`;
  if (el.dataset.sig === sig) return;
  el.dataset.sig = sig;
  const deco = el.querySelector(".deco")!;
  if (el.classList.contains("card")) {
    deco.innerHTML = `<span class="check">${icons.check}</span>${img.favorite ? `<span class="fav">${icons.heartSmall}</span>` : ""}`;
    el.querySelector(".caption")!.innerHTML = `<b>${esc(img.name)}</b><span>${dims(img)}</span>`;
  } else {
    el.style.background = img.palette[0] ?? "";
    deco.innerHTML =
      (img.kind !== "JPG" ? `<span class="badge">${img.kind}</span>` : "") +
      (img.favorite ? `<span class="fav">${icons.heartSmall}</span>` : "") +
      `<div class="meta"><b>${esc(img.name)}</b><span>${dims(img)}</span></div>`;
  }
}

function buildBlock(b: Block): HTMLElement {
  const el = document.createElement("div");
  el.style.top = `${b.top}px`;
  el.style.height = `${b.height}px`;
  if (b.head) {
    el.className = "group-head";
    el.innerHTML = `<h2>${esc(b.head.label)}</h2><span>${plural(b.head.count, "image")}</span>`;
  } else {
    el.className = "row";
    for (const p of b.items!) {
      const t = tileFor(p);
      tiles.set(p.img.id, t);
      el.append(t);
    }
  }
  return el;
}

function firstVisible(top: number): number {
  let lo = 0;
  let hi = blocks.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    const b = blocks[mid]!;
    if (b.top + b.height < top) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}

function mountVisible() {
  const top = content.scrollTop - 900;
  const bottom = content.scrollTop + content.clientHeight + 900;
  const keep = new Set<number>();
  for (let i = firstVisible(top); i < blocks.length && blocks[i]!.top <= bottom; i++) {
    keep.add(i);
    if (!mountedBlocks.has(i)) {
      const el = buildBlock(blocks[i]!);
      canvas.append(el);
      mountedBlocks.set(i, el);
    }
  }
  for (const [i, el] of mountedBlocks) {
    if (keep.has(i)) continue;
    el.remove();
    mountedBlocks.delete(i);
    for (const p of blocks[i]?.items ?? []) tiles.delete(p.img.id);
  }
}

function renderGrid() {
  const relayout = layoutDirty || content.clientWidth !== layoutWidth;
  if (relayout) {
    // Keep the image at the top of the view where it is.
    let anchorId: string | null = null;
    let offset = 0;
    const st = content.scrollTop;
    const focusPos = S.focus ? where.get(S.focus) : undefined;
    if (focusPos && focusPos.top >= st && focusPos.top < st + content.clientHeight) {
      anchorId = S.focus;
      offset = focusPos.top - st;
    } else {
      const b = blocks[firstVisible(st)];
      const firstItem = b?.items?.[0] ?? blocks[firstVisible(st) + 1]?.items?.[0];
      if (firstItem && st > 0) {
        anchorId = firstItem.img.id;
        offset = (where.get(anchorId)?.top ?? 0) - st;
      }
    }
    computeLayout();
    layoutDirty = false;
    // Tiles of the old layout are reused by the new one, so pictures
    // already loaded don't fade in again.
    for (const el of mountedBlocks.values()) el.remove();
    mountedBlocks.clear();
    const keepTiles = new Map(tiles);
    tiles.clear();
    for (const [id, el] of keepTiles) tiles.set(id, el);
    if (anchorId && where.has(anchorId)) content.scrollTop = where.get(anchorId)!.top - offset;
    mountVisible();
    for (const id of keepTiles.keys()) if (!where.has(id)) tiles.delete(id);
  } else {
    mountVisible();
    for (const [id, el] of tiles) {
      const img = S.images.get(id);
      if (img) decorate(el, img);
    }
  }
  renderEmpty();
}

content.addEventListener("scroll", () => {
  if (!frame) requestAnimationFrame(mountVisible);
}, { passive: true });
new ResizeObserver(() => {
  if (content.clientWidth !== layoutWidth) schedule("grid");
}).observe(content);

// Pictures fade in once loaded.
document.addEventListener("load", (e) => {
  const t = e.target as HTMLElement;
  if (t.tagName === "IMG") t.classList.add("loaded");
}, true);
document.addEventListener("error", (e) => {
  const t = e.target as HTMLElement;
  if (t.tagName !== "IMG") return;
  const tile = t.closest<HTMLElement>(".tile");
  if (tile && !tile.querySelector(".failed")) {
    const img = S.images.get(tile.dataset.id ?? "");
    tile.insertAdjacentHTML("beforeend", `<span class="failed">${img?.kind ?? "?"}</span>`);
  }
}, true);

// ---------------------------------------------------------------- empty states

function renderEmpty() {
  const empty = $("#empty");
  let html = "";
  if (isWelcome()) {
    html = `<div class="welcome-view">
      <div class="welcome-art" aria-hidden="true"><i class="card-a"></i><i class="card-b"></i><i class="card-c"></i><span class="plus">${icons.plus}</span></div>
      <h2>Bring your images in</h2>
      <p>Add a folder and Eagle indexes it where it is. Nothing is copied or moved, and new files show up as soon as they land.</p>
      <div class="actions"><button type="button" class="btn primary" data-act="add-folder">Add Folder… <kbd>⌘O</kbd></button></div>
      <div class="hint">${icons.drop}<span>Or drop folders anywhere in this window</span></div>
      <div class="features">
        <div><span class="dots"><i style="background:#E5484D"></i><i style="background:#F5C443"></i><i style="background:#3E7BFA"></i></span><strong>Filter by colour</strong><span>Every image gets a palette as it is indexed.</span></div>
        <div><span class="key">⌘F</span><strong>Find anything</strong><span>Search names, tags and folders as you type.</span></div>
        <div><span class="key">Space</span><strong>Look closer</strong><span>Open any image full-window and flip with the arrows.</span></div>
      </div>
    </div>`;
  } else if (S.loaded && list().length === 0) {
    const src = S.source;
    const folder = src.kind === "folder" ? S.folders.find((f) => f.id === src.id) : undefined;
    if (sourceImages().length && S.f.query && !hasFilters()) {
      html = `<h3>Nothing matches “${esc(S.f.query)}”</h3><p>Search looks at file names, tags and folder names.</p><button type="button" class="btn" data-act="clear-all">Clear Search</button>`;
    } else if (sourceImages().length && (hasFilters() || S.f.query)) {
      html = `<h3>No images match</h3><p>Try another colour or type, or clear the filters.</p><button type="button" class="btn" data-act="clear-all">Clear Filters</button>`;
    } else if (folder?.missing) {
      html = `<h3>Folder not found</h3><p>${esc(folder.path)} can't be read. If it is on a disk that isn't connected, connect it and look again.</p><div class="actions" style="display:flex;gap:8px"><button type="button" class="btn" data-act="rescan" data-v="${folder.id}">Look Again</button><button type="button" class="btn danger" data-act="remove-folder" data-v="${folder.id}">Remove from Library</button></div>`;
    } else if (folder && (folder.indexing || folder.count > 0)) {
      html = `<h3>Looking for images…</h3><p>${esc(folder.path)}</p>`;
    } else {
      const copy: Record<string, [string, string]> = {
        all: ["No images yet", "The folders you added have no images in them yet."],
        recent: ["Nothing recent", `No image was modified in the past ${RECENT_DAYS} days.`],
        favorites: ["No favorites yet", "Mark images with the heart in the inspector, or press F."],
        untagged: ["Everything is tagged", "Every image has at least one tag."],
        tag: ["No images with this tag", ""],
        folder: ["No images here", "Images you put in this folder show up here."],
      };
      const [h, p] = copy[src.kind]!;
      html = `<h3>${h}</h3>${p ? `<p>${p}</p>` : ""}`;
    }
  }
  empty.innerHTML = html;
  empty.hidden = !html;
}

$("#empty").addEventListener("click", (e) => {
  const el = (e.target as HTMLElement).closest<HTMLElement>("[data-act]");
  if (!el) return;
  const act = el.dataset.act;
  if (act === "add-folder") addFolder();
  else if (act === "clear-all") {
    S.f = emptyFilters();
    refresh();
  } else if (act === "rescan") Library.rescan(el.dataset.v!).catch(fail);
  else if (act === "remove-folder") Library.removeFolder(el.dataset.v!).catch(fail);
});

// ---------------------------------------------------------------- selection

function renderSelection() {
  for (const [id, el] of tiles) {
    el.classList.toggle("selected", S.selection.has(id));
    el.classList.toggle("focus", S.focus === id && !S.selection.has(id));
  }
}

function selectOnly(id: string) {
  S.selection = new Set([id]);
  S.anchor = S.focus = id;
  reveal(id);
  schedule("selection");
}

function toggleSelect(id: string) {
  if (S.selection.has(id)) S.selection.delete(id);
  else S.selection.add(id);
  S.anchor = S.focus = id;
  schedule("selection");
}

function selectRange(from: string, to: string) {
  const items = list();
  let a = items.findIndex((i) => i.id === from);
  let b = items.findIndex((i) => i.id === to);
  if (a < 0) a = b;
  if (a > b) [a, b] = [b, a];
  S.selection = new Set(items.slice(a, b + 1).map((i) => i.id));
  S.focus = to;
  reveal(to);
  schedule("selection");
}

function clearSelection() {
  if (!S.selection.size) return;
  S.selection.clear();
  schedule("selection");
}

function selected(): Image[] {
  return list().filter((i) => S.selection.has(i.id));
}

/** Scrolls the grid so an image shows. */
function reveal(id: string) {
  const p = where.get(id);
  if (!p) return;
  const st = content.scrollTop;
  const h = content.clientHeight;
  if (p.top < st + 8) content.scrollTop = p.top - (blocks[p.block - 1]?.head ? HEAD + 8 : 16);
  else if (p.top + p.h > st + h - 8) content.scrollTop = p.top + p.h - h + 20;
  mountVisible();
}

/** Moves the focus with the arrow keys; with ⇧ it extends the selection. */
function move(dir: "left" | "right" | "up" | "down", extend: boolean) {
  const items = list();
  if (!items.length) return;
  const cur = S.focus && where.has(S.focus) ? S.focus : null;
  let next: string | undefined;
  if (!cur) next = items[0]!.id;
  else if (dir === "left" || dir === "right") {
    const i = items.findIndex((x) => x.id === cur);
    next = items[clamp(i + (dir === "right" ? 1 : -1), 0, items.length - 1)]?.id;
  } else {
    const p = where.get(cur)!;
    const center = p.x + p.w / 2;
    let i = p.block + (dir === "down" ? 1 : -1);
    while (blocks[i] && !blocks[i]!.items) i += dir === "down" ? 1 : -1;
    const row = blocks[i]?.items;
    if (row) {
      let best = row[0]!;
      for (const q of row) if (Math.abs(q.x + q.w / 2 - center) < Math.abs(best.x + best.w / 2 - center)) best = q;
      next = best.img.id;
    }
  }
  if (!next) return;
  if (extend && S.anchor) selectRange(S.anchor, next);
  else selectOnly(next);
}

content.addEventListener("mousedown", (e) => {
  if (e.button !== 0) return;
  const t = e.target as HTMLElement;
  if (t.closest("#empty")) return;
  const el = t.closest<HTMLElement>("[data-id]");
  content.focus({ preventScroll: true });
  if (!el) return clearSelection();
  const id = el.dataset.id!;
  if (e.shiftKey && S.anchor) selectRange(S.anchor, id);
  else if (e.metaKey) toggleSelect(id);
  else if (!S.selection.has(id) || e.detail > 1) selectOnly(id);
  else {
    S.focus = S.anchor = id;
    schedule("selection");
  }
});
content.addEventListener("click", (e) => {
  // A plain click inside a multiple selection narrows it to that image.
  const el = (e.target as HTMLElement).closest<HTMLElement>("[data-id]");
  if (el && !e.shiftKey && !e.metaKey && S.selection.size > 1 && e.detail === 1) selectOnly(el.dataset.id!);
});
content.addEventListener("dblclick", (e) => {
  const el = (e.target as HTMLElement).closest<HTMLElement>("[data-id]");
  if (el) openViewer(el.dataset.id!);
});
content.addEventListener("contextmenu", (e) => {
  const el = (e.target as HTMLElement).closest<HTMLElement>("[data-id]");
  if (!el) return;
  e.preventDefault();
  const id = el.dataset.id!;
  if (!S.selection.has(id)) selectOnly(id);
  flush();
  const sel = selected();
  const one = sel.length === 1 ? sel[0]! : null;
  const allFav = sel.every((i) => i.favorite);
  openMenu({ x: e.clientX, y: e.clientY }, [
    { label: sel.length > 1 ? `Open ${sel.length} in Viewer` : "Open in Viewer", kbd: "Space", run: () => openViewer(id) },
    { label: "Quick Look", kbd: "⌘Y", run: () => Library.quickLook(id).catch(fail) },
    { label: "Open with Default App", disabled: !one, run: () => Library.openExternally(id).catch(fail) },
    { label: "Show in Finder", kbd: "⇧⌘R", run: () => Library.reveal(id).catch(fail) },
    "-",
    { label: allFav ? "Remove from Favorites" : "Add to Favorites", kbd: "F", run: () => setFavorite(sel, !allFav) },
    { label: sel.length > 1 ? "Copy Paths" : "Copy Path", run: () => copyText(sel.map((i) => i.path).join("\n")) },
    "-",
    { label: "Move to Trash…", kbd: "⌘⌫", danger: true, run: () => trash() },
  ]);
});

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    toast("Copied");
  } catch (e) {
    fail(e);
  }
}

// ---------------------------------------------------------------- editing

function setRating(imgs: Image[], rating: number) {
  if (!imgs.length) return;
  for (const i of imgs) i.rating = rating;
  Library.setRating(imgs.map((i) => i.id), rating).catch(fail);
  refresh();
}

function setFavorite(imgs: Image[], favorite: boolean) {
  if (!imgs.length) return;
  for (const i of imgs) i.favorite = favorite;
  Library.setFavorite(imgs.map((i) => i.id), favorite).catch(fail);
  refresh();
  if (viewer.open) renderViewerChrome();
}

function addTag(imgs: Image[], tag: string) {
  tag = tag.trim().toLowerCase();
  if (!tag || !imgs.length) return;
  for (const i of imgs) if (!i.tags.includes(tag)) i.tags = [...i.tags, tag];
  Library.addTag(imgs.map((i) => i.id), tag).catch(fail);
  refocusTagInput = true;
  refresh();
}

function removeTag(imgs: Image[], tag: string) {
  for (const i of imgs) i.tags = i.tags.filter((t) => t !== tag);
  Library.removeTag(imgs.map((i) => i.id), tag).catch(fail);
  refresh();
}

async function trash() {
  const ids = [...S.selection];
  if (!ids.length) return;
  try {
    const moved = await Library.trash(ids);
    if (!moved.length) return;
    toast(`Moved ${plural(moved.length, "image")} to the Trash`, {
      label: "Undo",
      run: () => Library.restore(moved).catch(fail),
    });
  } catch (e) {
    fail(e);
  }
}

async function addFolder() {
  try {
    added(await Library.addFolder());
  } catch (e) {
    fail(e);
  }
}

async function addPaths(paths: string[]) {
  try {
    added(await Library.addPaths(paths));
  } catch (e) {
    fail(e);
  }
}

function added(folders: Folder[]) {
  if (!folders.length) return;
  for (const f of folders) if (!S.folders.some((x) => x.id === f.id)) S.folders.push(f);
  setSource({ kind: "folder", id: folders[0]!.id, dir: "" });
  toast(folders.length === 1 ? `Added ${folders[0]!.name}` : `Added ${folders.length} folders`);
}

// ---------------------------------------------------------------- inspector

let refocusTagInput = false;

function renderInspector() {
  const el = $("#inspector");
  if (!S.inspector || isWelcome()) {
    el.innerHTML = "";
    return;
  }
  const sel = selected();
  let html: string;
  if (sel.length === 1) html = inspectOne(sel[0]!);
  else if (sel.length > 1) html = inspectMany(sel);
  else html = inspectView();
  el.innerHTML = `<div class="ins">${html}</div>`;
  if (refocusTagInput) {
    refocusTagInput = false;
    el.querySelector<HTMLInputElement>("input[data-act=tag]")?.focus();
  }
}

function paletteHTML(colors: string[], thin = false) {
  if (!colors.length) return `<div class="palette pending${thin ? " thin" : ""}" title="Finding colours…"></div>`;
  return `<div class="palette${thin ? " thin" : ""}">${colors
    .map((hex) => {
      const n = parseInt(hex.slice(1), 16);
      const lum = 0.299 * ((n >> 16) & 255) + 0.587 * ((n >> 8) & 255) + 0.114 * (n & 255);
      return `<button type="button" data-act="palette" data-v="${hex}" style="background:${hex};color:${lum > 150 ? "rgba(0,0,0,.7)" : "rgba(255,255,255,.9)"}" title="Find images with this colour"><span>${hex}</span></button>`;
    })
    .join("")}</div>`;
}

function starsHTML(rating: number, mixed = false) {
  return `<div class="stars${mixed ? " mixed" : ""}" role="group" aria-label="Rating">${[1, 2, 3, 4, 5]
    .map((n) => `<button type="button" data-act="rate" data-v="${n}" class="${n <= rating ? "on" : ""}" aria-label="${n} star${n > 1 ? "s" : ""}">${icons.star}</button>`)
    .join("")}</div>`;
}

function folderPath(img: Image) {
  const name = folderName(img.folder);
  return img.dir ? `${name} › ${img.dir.replaceAll("/", " › ")}` : name;
}

function inspectOne(img: Image) {
  const ar = img.width ? `${img.width} / ${img.height}` : "3 / 2";
  // Tall images are kept short, so the details stay in view.
  const maxW = Math.round(240 * aspect(img));
  return `
    <div class="preview" style="aspect-ratio:${ar};width:min(100%, ${maxW}px);margin-inline:auto;background:${img.palette[0] ?? "var(--tile-bg)"}">
      <img alt="" src="${thumbURL(img)}">
      <button type="button" data-act="viewer" aria-label="Open in viewer" title="Open in viewer (Space)">${icons.expand}</button>
    </div>
    <div class="ins-name" title="${esc(img.name)}">${esc(img.name)}</div>
    <div class="ins-sub">${img.kind} · ${dims(img)} · ${fmtBytes(img.size)}</div>
    <div class="ins-section" style="padding-top:16px">${paletteHTML(img.palette)}</div>
    <div class="ins-section">
      <div class="ins-row"><h4>Rating</h4><div style="display:flex;align-items:center;gap:4px">${starsHTML(img.rating)}<button type="button" class="fav-btn${img.favorite ? " on" : ""}" data-act="fav" aria-pressed="${img.favorite}" aria-label="Favorite" title="Favorite (F)">${img.favorite ? icons.heartFill : icons.heart}</button></div></div>
      <h4>Tags</h4>
      <div class="tags">${img.tags.map((t) => `<span class="t">${esc(t)}<button type="button" data-act="untag" data-v="${esc(t)}" aria-label="Remove tag ${esc(t)}">${icons.closeSmall}</button></span>`).join("")}<input data-act="tag" placeholder="Add tag" aria-label="Add tag" spellcheck="false"></div>
    </div>
    <div class="ins-section">
      <dl class="info">
        <dt>Dimensions</dt><dd>${img.width ? `${dims(img)} px` : "—"}</dd>
        <dt>File size</dt><dd>${fmtBytes(img.size)}</dd>
        <dt>Kind</dt><dd>${KIND_LABEL[img.kind]}</dd>
        <dt>Modified</dt><dd>${fmtDate(img.modified)}</dd>
        <dt>Folder</dt><dd title="${esc(img.path)}">${esc(folderPath(img))}</dd>
      </dl>
    </div>
    <div class="ins-section"><div class="actions"><button type="button" class="btn primary" data-act="viewer">Open</button><button type="button" class="btn" data-act="reveal">Show in Finder</button></div></div>`;
}

function inspectMany(sel: Image[]) {
  const top = sel.slice(0, 3).reverse();
  const bytes = sel.reduce((a, i) => a + i.size, 0);
  const kinds = [...new Set(sel.map((i) => i.kind))];
  // The palette the images share: their colors, weighted by coverage.
  const weight = new Map<string, number>();
  for (const img of sel) img.palette.forEach((hex, i) => weight.set(hex, (weight.get(hex) ?? 0) + (img.weights[i] ?? 0)));
  const shared: string[] = [];
  for (const [hex] of [...weight].sort((a, b) => b[1] - a[1])) {
    if (shared.length === 5) break;
    if (!shared.some((h) => colorDistance(h, hex) < 40)) shared.push(hex);
  }
  const ratings = new Set(sel.map((i) => i.rating));
  const rating = ratings.size === 1 ? sel[0]!.rating : 0;
  const tagCount = new Map<string, number>();
  for (const img of sel) for (const t of img.tags) tagCount.set(t, (tagCount.get(t) ?? 0) + 1);
  const tags = [...tagCount].sort((a, b) => b[1] - a[1]);
  const allFav = sel.every((i) => i.favorite);
  return `
    <div class="stack" aria-hidden="true">${top.map((i) => `<img alt="" src="${thumbURL(i)}" style="background:${i.palette[0] ?? "var(--tile-bg)"}">`).join("")}<span class="n">${fmtInt(sel.length)}</span></div>
    <div class="ins-name">${plural(sel.length, "image")} selected</div>
    <div class="ins-sub">${kinds.slice(0, 3).join(", ")}${kinds.length > 3 ? "…" : ""} · ${fmtBytes(bytes)} total</div>
    <div class="ins-section" style="padding-top:16px"><h4>Shared palette</h4>${paletteHTML(shared, true)}</div>
    <div class="ins-section">
      <div class="ins-row"><h4>Rating${ratings.size > 1 ? " · mixed" : ""}</h4><div style="display:flex;align-items:center;gap:4px">${starsHTML(rating, ratings.size > 1)}<button type="button" class="fav-btn${allFav ? " on" : ""}" data-act="fav" aria-label="Favorite all">${allFav ? icons.heartFill : icons.heart}</button></div></div>
      <h4>Tags</h4>
      <div class="tags">${tags.map(([t, n]) => `<span class="t${n < sel.length ? " partial" : ""}" title="${n < sel.length ? `On ${n} of ${sel.length}` : "On all"}">${esc(t)}${n < sel.length ? ` · ${n}` : ""}<button type="button" data-act="untag" data-v="${esc(t)}" aria-label="Remove tag ${esc(t)} from all">${icons.closeSmall}</button></span>`).join("")}<input data-act="tag" placeholder="Tag all" aria-label="Add a tag to all" spellcheck="false"></div>
    </div>
    <div class="ins-section actions-col">
      <button type="button" class="btn primary" data-act="viewer">Open ${fmtInt(sel.length)} in Viewer</button>
      <div class="actions"><button type="button" class="btn" data-act="reveal">Show in Finder</button><button type="button" class="btn danger" data-act="trash">Move to Trash…</button></div>
    </div>`;
}

function inspectView() {
  const items = list();
  const bytes = items.reduce((a, i) => a + i.size, 0);
  const byKind = new Map<Kind, number>();
  const byColor = new Map<string, number>();
  for (const i of items) {
    byKind.set(i.kind, (byKind.get(i.kind) ?? 0) + 1);
    for (const c of i.colors) byColor.set(c, (byColor.get(c) ?? 0) + 1);
  }
  const kinds = KINDS.filter((k) => byKind.get(k));
  const maxColor = Math.max(1, ...byColor.values());
  const colors = COLORS.filter((c) => byColor.get(c.name)).sort((a, b) => byColor.get(b.name)! - byColor.get(a.name)!);
  return `
    <h4 style="margin:0;font-size:11px;font-weight:600;color:var(--text-2)">${esc(sourceTitle())}</h4>
    <div class="big-num">${fmtInt(items.length)}</div>
    <div class="ins-sub">${items.length === 1 ? "image" : "images"} · ${fmtBytes(bytes)}</div>
    ${kinds.length ? `<div class="ins-section" style="margin-top:8px"><h4>Types</h4>
      <div class="breakdown">${kinds.map((k) => `<i style="flex:${byKind.get(k)};background:${KIND_COLOR[k]}"></i>`).join("")}</div>
      <div class="legend">${kinds.map((k) => `<div><i style="background:${KIND_COLOR[k]}"></i>${k}<span>${fmtInt(byKind.get(k)!)}</span></div>`).join("")}</div></div>` : ""}
    ${colors.length ? `<div class="ins-section"><h4>Colours</h4><div class="color-bars">${colors
      .map((c) => `<button type="button" data-act="color" data-v="${c.name}"><span class="sw" style="background:${c.hex}"></span><span class="lbl">${c.name}</span><span class="track"><i style="width:${(byColor.get(c.name)! / maxColor) * 100}%;background:${c.hex}"></i></span><span class="n">${fmtInt(byColor.get(c.name)!)}</span></button>`)
      .join("")}</div></div>` : ""}
    <div class="ins-section"><p style="margin:0;font-size:12px;color:var(--text-2);line-height:1.5">Select an image to see its details. Double-click or press Space to open it.</p></div>`;
}

$("#inspector").addEventListener("click", (e) => {
  const el = (e.target as HTMLElement).closest<HTMLElement>("[data-act]");
  if (!el) return;
  const sel = selected();
  const v = el.dataset.v ?? "";
  switch (el.dataset.act) {
    case "viewer": openViewer(); break;
    case "reveal": if (sel[0]) Library.reveal(sel[0].id).catch(fail); break;
    case "trash": trash(); break;
    case "rate": {
      const n = Number(v);
      const same = sel.length === 1 && sel[0]!.rating === n;
      setRating(sel, same ? 0 : n);
      break;
    }
    case "fav": setFavorite(sel, !sel.every((i) => i.favorite)); break;
    case "untag": removeTag(sel, v); break;
    case "palette": {
      const family = colorFamilyOf(v);
      S.f.colors = new Set([family]);
      refresh();
      break;
    }
    case "color": toggleColor(v); break;
  }
});
$("#inspector").addEventListener("keydown", (e) => {
  const input = e.target as HTMLInputElement;
  if (input.dataset.act !== "tag") return;
  if (e.key === "Enter") {
    addTag(selected(), input.value);
  } else if (e.key === "Escape") {
    input.value = "";
    input.blur();
    e.stopPropagation();
  }
});

function colorDistance(a: string, b: string): number {
  const x = parseInt(a.slice(1), 16);
  const y = parseInt(b.slice(1), 16);
  return Math.hypot(((x >> 16) & 255) - ((y >> 16) & 255), ((x >> 8) & 255) - ((y >> 8) & 255), (x & 255) - (y & 255));
}

/** Mirrors colorFamily in imaging.go, for colors clicked in a palette. */
function colorFamilyOf(hex: string): string {
  const n = parseInt(hex.slice(1), 16);
  const r = ((n >> 16) & 255) / 255, g = ((n >> 8) & 255) / 255, b = (n & 255) / 255;
  const mx = Math.max(r, g, b), mn = Math.min(r, g, b);
  const l = (mx + mn) / 2, d = mx - mn;
  let h = 0, s = 0;
  if (d > 0) {
    s = d / (1 - Math.abs(2 * l - 1));
    h = mx === r ? ((g - b) / d) % 6 : mx === g ? (b - r) / d + 2 : (r - g) / d + 4;
    h *= 60;
    if (h < 0) h += 360;
  }
  if (l < 0.13) return "black";
  if (l > 0.86 && d < 0.2) return "white";
  if (d < 0.1 || s < 0.12) return l > 0.78 ? "white" : l < 0.24 ? "black" : "gray";
  if ((h < 45 || h >= 340) && l < 0.46 && s < 0.65) return "brown";
  if (h >= 345 || h < 14) return "red";
  if (h < 42) return "orange";
  if (h < 66) return "yellow";
  if (h < 160) return "green";
  if (h < 195) return "teal";
  if (h < 252) return "blue";
  if (h < 292) return "purple";
  return "pink";
}

// ---------------------------------------------------------------- status bar

function renderStatus() {
  const items = list();
  const n = S.selection.size;
  const left = n
    ? `${fmtInt(n)} of ${plural(items.length, "image")} selected · ${fmtBytes(selected().reduce((a, i) => a + i.size, 0))}`
    : plural(items.length, "image");
  $("#statusbar").innerHTML = `<span>${left}</span><span class="spacer"></span><span>Space to view · ⌘I inspector</span>`;
}

// ---------------------------------------------------------------- viewer

const viewerEl = $("#viewer");
const viewer = {
  open: false,
  ids: [] as string[],
  i: 0,
  zoom: 0, // 0 fits the image to the window; otherwise its scale
  panX: 0,
  panY: 0,
  info: true,
  natural: { w: 0, h: 0 },
};

function openViewer(startId?: string) {
  const items = list();
  if (!items.length) return;
  const sel = selected();
  const ids = sel.length > 1 ? sel.map((i) => i.id) : items.map((i) => i.id);
  const start = startId ?? S.focus ?? sel[0]?.id ?? ids[0]!;
  viewer.ids = ids;
  viewer.i = Math.max(0, ids.indexOf(start));
  viewer.open = true;
  closePopover();
  viewerEl.innerHTML = `
    <img class="ambient" alt="" aria-hidden="true">
    <div class="vignette" aria-hidden="true"></div>
    <header>
      <div class="side"></div>
      <div class="title"><b></b><span></span></div>
      <div class="side right">
        <button type="button" class="v-btn fav" data-act="fav" aria-label="Favorite" title="Favorite (F)"></button>
        <button type="button" class="v-btn" data-act="info" aria-label="Show info" title="Info (I)">${icons.info}</button>
        <button type="button" class="v-btn" data-act="close" aria-label="Close viewer" title="Close (Esc)">${icons.close}</button>
      </div>
    </header>
    <div class="stage">
      <img class="photo" alt="" draggable="false">
      <button type="button" class="nav prev" data-act="prev" aria-label="Previous image">${icons.chevronLeft}</button>
      <button type="button" class="nav next" data-act="next" aria-label="Next image">${icons.chevronRight}</button>
      <div class="zoombar">
        <button type="button" class="v-btn" data-act="zoom-out" aria-label="Zoom out">${icons.minus}</button>
        <span class="pct"></span>
        <button type="button" class="v-btn" data-act="zoom-in" aria-label="Zoom in">${icons.plusLg}</button>
        <span class="sep"></span>
        <button type="button" class="v-btn" data-act="fit">Fit</button>
        <button type="button" class="v-btn" data-act="actual">1:1</button>
      </div>
      <aside class="hud" aria-label="Image info"></aside>
    </div>
    <nav class="filmstrip" aria-label="Filmstrip"></nav>`;
  viewerEl.hidden = false;
  showCurrent();
}

function closeViewer() {
  viewer.open = false;
  viewerEl.hidden = true;
  viewerEl.innerHTML = "";
  const id = viewer.ids[viewer.i];
  if (id && S.selection.size <= 1) selectOnly(id);
  content.focus({ preventScroll: true });
}

const current = () => S.images.get(viewer.ids[viewer.i] ?? "");

function showCurrent() {
  const img = current();
  if (!img) return closeViewer();
  viewer.zoom = 0;
  viewer.panX = viewer.panY = 0;
  viewer.natural = { w: img.width, h: img.height };
  const photo = viewerEl.querySelector<HTMLImageElement>("img.photo")!;
  viewerEl.querySelector<HTMLImageElement>(".ambient")!.src = thumbURL(img);
  // The thumbnail shows at once; the full image replaces it once loaded.
  photo.src = thumbURL(img);
  const full = new window.Image();
  full.onload = () => {
    if (current() !== img) return;
    // A preview of a HEIC or RAW file can be smaller than the original:
    // zoom is measured against the original's size when it is known.
    if (!img.width) viewer.natural = { w: full.naturalWidth, h: full.naturalHeight };
    photo.src = full.src;
    layoutPhoto(false);
  };
  full.src = fullURL(img);
  renderViewerChrome();
  renderFilmstrip();
  layoutPhoto(false);
  if (S.selection.size <= 1) {
    S.selection = new Set([img.id]);
    S.focus = S.anchor = img.id;
  }
}

function renderViewerChrome() {
  const img = current();
  if (!img) return;
  viewerEl.querySelector(".title b")!.textContent = img.name;
  viewerEl.querySelector(".title span")!.textContent = `${fmtInt(viewer.i + 1)} of ${fmtInt(viewer.ids.length)} · ${fmtDate(img.modified)}`;
  const fav = viewerEl.querySelector<HTMLElement>("[data-act=fav]")!;
  fav.classList.toggle("on", img.favorite);
  fav.innerHTML = img.favorite ? icons.heartFill : icons.heart;
  viewerEl.querySelector("[data-act=info]")!.classList.toggle("on", viewer.info);
  viewerEl.querySelector<HTMLButtonElement>(".nav.prev")!.disabled = viewer.i === 0;
  viewerEl.querySelector<HTMLButtonElement>(".nav.next")!.disabled = viewer.i === viewer.ids.length - 1;
  const hud = viewerEl.querySelector<HTMLElement>(".hud")!;
  hud.hidden = !viewer.info;
  hud.innerHTML = `
    <div><b>${esc(img.name)}</b><div class="sub">${img.kind} · ${dims(img)} · ${fmtBytes(img.size)}</div></div>
    ${img.palette.length ? `<div class="pal">${img.palette.map((h) => `<i style="background:${h}"></i>`).join("")}</div>` : ""}
    <dl>
      <dt>Rating</dt><dd class="${img.rating ? "rate" : ""}">${img.rating ? stars(img.rating) : "Not rated"}</dd>
      ${img.tags.length ? `<dt>Tags</dt><dd>${esc(img.tags.join(", "))}</dd>` : ""}
      <dt>Folder</dt><dd title="${esc(img.path)}">${esc(folderPath(img))}</dd>
    </dl>`;
}

function renderFilmstrip() {
  const strip = viewerEl.querySelector<HTMLElement>(".filmstrip")!;
  const from = Math.max(0, viewer.i - 40);
  const to = Math.min(viewer.ids.length, viewer.i + 41);
  let html = "";
  for (let k = from; k < to; k++) {
    const img = S.images.get(viewer.ids[k]!);
    if (!img) continue;
    html += `<button type="button" data-act="go" data-v="${k}" class="${k === viewer.i ? "on" : ""}" aria-label="${esc(img.name)}" style="aspect-ratio:${aspect(img)}"><img alt="" src="${thumbURL(img)}"></button>`;
  }
  strip.innerHTML = html;
  const on = strip.querySelector<HTMLElement>(".on");
  if (on) {
    strip.style.scrollBehavior = "auto";
    strip.scrollLeft = on.offsetLeft + on.offsetWidth / 2 - strip.clientWidth / 2;
    strip.style.scrollBehavior = "";
  }
}

function fitScale() {
  const stage = viewerEl.querySelector<HTMLElement>(".stage")!;
  const { w, h } = viewer.natural;
  if (!w || !h) return 1;
  // Room for the arrows at the sides and the zoom bar below.
  return Math.min((stage.clientWidth - 180) / w, (stage.clientHeight - 150) / h, 1);
}
const scaleNow = () => (viewer.zoom || fitScale());

function layoutPhoto(animate = true) {
  const stage = viewerEl.querySelector<HTMLElement>(".stage");
  const photo = viewerEl.querySelector<HTMLImageElement>("img.photo");
  if (!stage || !photo) return;
  let { w, h } = viewer.natural;
  if (!w || !h) {
    w = photo.naturalWidth || 1200;
    h = photo.naturalHeight || 800;
    viewer.natural = { w, h };
  }
  const scale = scaleNow();
  const pw = w * scale;
  const ph = h * scale;
  const maxX = Math.max(0, (pw - stage.clientWidth) / 2);
  const maxY = Math.max(0, (ph - stage.clientHeight) / 2);
  viewer.panX = clamp(viewer.panX, -maxX, maxX);
  viewer.panY = clamp(viewer.panY, -maxY, maxY);
  stage.classList.toggle("animate", animate);
  stage.classList.toggle("zoomed", viewer.zoom > 0 && (maxX > 0 || maxY > 0));
  photo.style.width = `${pw}px`;
  photo.style.height = `${ph}px`;
  photo.style.transform = `translate(calc(-50% + ${viewer.panX}px), calc(-50% + ${viewer.panY}px))`;
  viewerEl.querySelector(".pct")!.textContent = `${Math.round(scale * 100)}%`;
  viewerEl.querySelector("[data-act=fit]")!.classList.toggle("on", !viewer.zoom);
}

function zoomTo(scale: number, cx = 0, cy = 0, animate = true) {
  const fit = fitScale();
  const before = scaleNow();
  const next = clamp(scale, Math.min(fit, 1) * 0.999, 8);
  if (next <= fit) {
    viewer.zoom = 0;
    viewer.panX = viewer.panY = 0;
  } else {
    // Keep the point under the cursor where it is.
    viewer.panX = (viewer.panX - cx) * (next / before) + cx;
    viewer.panY = (viewer.panY - cy) * (next / before) + cy;
    viewer.zoom = next;
  }
  layoutPhoto(animate);
}

function go(i: number) {
  if (i < 0 || i >= viewer.ids.length || i === viewer.i) return;
  viewer.i = i;
  showCurrent();
}

viewerEl.addEventListener("click", (e) => {
  const el = (e.target as HTMLElement).closest<HTMLElement>("[data-act]");
  if (!el) return;
  const img = current();
  switch (el.dataset.act) {
    case "close": closeViewer(); break;
    case "prev": go(viewer.i - 1); break;
    case "next": go(viewer.i + 1); break;
    case "go": go(Number(el.dataset.v)); break;
    case "info": viewer.info = !viewer.info; renderViewerChrome(); break;
    case "fav": if (img) setFavorite([img], !img.favorite); break;
    case "zoom-in": zoomTo(scaleNow() * 1.5); break;
    case "zoom-out": zoomTo(scaleNow() / 1.5); break;
    case "fit": zoomTo(0); break;
    case "actual": zoomTo(1); break;
  }
});

viewerEl.addEventListener("dblclick", (e) => {
  if (!(e.target as HTMLElement).matches("img.photo")) return;
  const stage = viewerEl.querySelector<HTMLElement>(".stage")!.getBoundingClientRect();
  const cx = e.clientX - stage.left - stage.width / 2;
  const cy = e.clientY - stage.top - stage.height / 2;
  if (viewer.zoom) zoomTo(0);
  else zoomTo(Math.max(1, fitScale() * 2), cx, cy);
});

viewerEl.addEventListener("wheel", (e) => {
  if (!viewer.open || !(e.target as HTMLElement).closest(".stage")) return;
  e.preventDefault();
  const stage = viewerEl.querySelector<HTMLElement>(".stage")!.getBoundingClientRect();
  if (e.ctrlKey) {
    // Pinch on the trackpad.
    const cx = e.clientX - stage.left - stage.width / 2;
    const cy = e.clientY - stage.top - stage.height / 2;
    zoomTo(scaleNow() * Math.exp(-e.deltaY * 0.01), cx, cy, false);
  } else if (viewer.zoom) {
    viewer.panX -= e.deltaX;
    viewer.panY -= e.deltaY;
    layoutPhoto(false);
  }
}, { passive: false });

let drag: { x: number; y: number; px: number; py: number } | null = null;
viewerEl.addEventListener("pointerdown", (e) => {
  const t = e.target as HTMLElement;
  if (!viewer.zoom || !t.matches("img.photo")) return;
  drag = { x: e.clientX, y: e.clientY, px: viewer.panX, py: viewer.panY };
  t.setPointerCapture(e.pointerId);
  viewerEl.querySelector(".stage")!.classList.add("dragging");
});
viewerEl.addEventListener("pointermove", (e) => {
  if (!drag) return;
  viewer.panX = drag.px + e.clientX - drag.x;
  viewer.panY = drag.py + e.clientY - drag.y;
  layoutPhoto(false);
});
viewerEl.addEventListener("pointerup", () => {
  drag = null;
  viewerEl.querySelector(".stage")?.classList.remove("dragging");
});
window.addEventListener("resize", () => {
  if (viewer.open) layoutPhoto(false);
});

function viewerKey(e: KeyboardEvent) {
  const img = current();
  switch (e.key) {
    case "Escape":
    case " ":
      closeViewer();
      break;
    case "ArrowLeft":
      go(viewer.i - 1);
      break;
    case "ArrowRight":
      go(viewer.i + 1);
      break;
    case "+":
    case "=":
      zoomTo(scaleNow() * 1.5);
      break;
    case "-":
      zoomTo(scaleNow() / 1.5);
      break;
    case "0":
      zoomTo(0);
      break;
    case "1":
      zoomTo(1);
      break;
    case "i":
      viewer.info = !viewer.info;
      renderViewerChrome();
      break;
    case "f":
      if (img) setFavorite([img], !img.favorite);
      break;
    default:
      return;
  }
  e.preventDefault();
}

// ---------------------------------------------------------------- keyboard & commands

function runCommand(cmd: string) {
  if (viewer.open) {
    if (cmd === "zoomIn") return zoomTo(scaleNow() * 1.5);
    if (cmd === "zoomOut") return zoomTo(scaleNow() / 1.5);
    if (cmd === "openViewer") return closeViewer();
  }
  const focusId = S.focus ?? [...S.selection][0];
  switch (cmd) {
    case "addFolder": addFolder(); break;
    case "openViewer": openViewer(); break;
    case "quickLook": if (focusId) Library.quickLook(focusId).catch(fail); break;
    case "reveal": if (focusId) Library.reveal(focusId).catch(fail); break;
    case "trash": trash(); break;
    case "layoutJustified": setLayout("justified"); break;
    case "layoutGrid": setLayout("grid"); break;
    case "zoomIn": setZoom(S.zoom + 40); break;
    case "zoomOut": setZoom(S.zoom - 40); break;
    case "toggleFilters":
      S.filterbar = !S.filterbar;
      savePrefs();
      schedule("chrome");
      break;
    case "toggleInspector": toggleInspector(); break;
    case "find":
      if (viewer.open) closeViewer();
      search.focus();
      search.select();
      break;
  }
}
events.command.on(runCommand);

// Outside MyGo there is no menu bar, so the page takes its shortcuts.
const BROWSER_SHORTCUTS: Record<string, string> = {
  o: "addFolder", f: "find", i: "toggleInspector", "1": "layoutJustified", "2": "layoutGrid",
  "=": "zoomIn", "-": "zoomOut", Backspace: "trash", y: "quickLook",
};

document.addEventListener("keydown", (e) => {
  if (viewer.open) return viewerKey(e);
  const t = e.target as HTMLElement;
  const typing = t.matches("input, textarea, [contenteditable]");
  if (e.key === "Escape") {
    if (pop) closePopover();
    else if (typing) t.blur();
    else clearSelection();
    return;
  }
  if (typing) return;
  const mod = e.metaKey || e.ctrlKey;
  if (mod && !isMyGo() && BROWSER_SHORTCUTS[e.key]) {
    e.preventDefault();
    return runCommand(BROWSER_SHORTCUTS[e.key]!);
  }
  if (mod && e.key.toLowerCase() === "a") {
    e.preventDefault();
    S.selection = new Set(list().map((i) => i.id));
    schedule("selection");
    return;
  }
  if (mod || e.altKey) return;
  switch (e.key) {
    case "ArrowLeft": move("left", e.shiftKey); break;
    case "ArrowRight": move("right", e.shiftKey); break;
    case "ArrowUp": move("up", e.shiftKey); break;
    case "ArrowDown": move("down", e.shiftKey); break;
    case " ":
    case "Enter":
      if (S.selection.size || S.focus) openViewer();
      break;
    case "f":
      setFavorite(selected(), !selected().every((i) => i.favorite));
      break;
    default:
      if (/^[0-5]$/.test(e.key)) {
        setRating(selected(), Number(e.key));
        break;
      }
      return;
  }
  e.preventDefault();
});

// ---------------------------------------------------------------- toast

const toastEl = $("#toast");
let toastTimer = 0;
function toast(message: string, action?: { label: string; run: () => void }) {
  toastEl.innerHTML = `<span>${esc(message)}</span>${action ? `<button type="button">${esc(action.label)}</button>` : ""}`;
  toastEl.classList.toggle("has-action", !!action);
  toastEl.classList.add("on");
  const btn = toastEl.querySelector("button");
  if (btn && action) btn.onclick = () => {
    toastEl.classList.remove("on");
    action.run();
  };
  toastLife = action ? 10000 : 3500;
  armToast();
}
let toastLife = 3500;
function armToast() {
  clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => toastEl.classList.remove("on"), toastLife);
}
// The toast waits while the pointer is on it, so Undo stays reachable.
toastEl.addEventListener("mouseenter", () => clearTimeout(toastTimer));
toastEl.addEventListener("mouseleave", () => {
  if (toastEl.classList.contains("on")) armToast();
});

// ---------------------------------------------------------------- drag and drop

const dropzone = $("#dropzone");
let dragDepth = 0;
const hasFiles = (e: DragEvent) => [...(e.dataTransfer?.types ?? [])].includes("Files");
window.addEventListener("dragenter", (e) => {
  if (!hasFiles(e)) return;
  dragDepth++;
  dropzone.classList.add("on");
});
window.addEventListener("dragleave", () => {
  dragDepth = Math.max(0, dragDepth - 1);
  if (!dragDepth) dropzone.classList.remove("on");
});
window.addEventListener("dragover", (e) => {
  if (hasFiles(e)) e.preventDefault();
});
window.addEventListener("drop", (e) => {
  e.preventDefault();
  dragDepth = 0;
  dropzone.classList.remove("on");
});
if (isMyGo()) onFileDrop(({ paths }) => addPaths(paths));

// ---------------------------------------------------------------- library events

events.folders.on((folders) => {
  const before = S.folders.length;
  S.folders = folders;
  if (S.source.kind === "folder" && !folders.some((f) => S.source.kind === "folder" && f.id === S.source.id)) S.source = { kind: "all" };
  if (!before !== !folders.length) refresh();
  else schedule("chrome", "sidebar", "toolbar", "grid");
});

events.images.on(({ folder, images }) => {
  for (const [id, img] of S.images) if (img.folder === folder) S.images.delete(id);
  for (const img of images) S.images.set(img.id, img);
  refresh();
});

events.folderRemoved.on((folder) => {
  for (const [id, img] of S.images) if (img.folder === folder) S.images.delete(id);
  S.folders = S.folders.filter((f) => f.id !== folder);
  if (S.source.kind === "folder" && S.source.id === folder) S.source = { kind: "all" };
  refresh();
});

events.imageUpdates.on((updates) => {
  let resized = false;
  let selectedChanged = false;
  for (const u of updates) {
    const img = S.images.get(u.id);
    if (!img) continue;
    if (img.width !== u.width || img.height !== u.height) resized = true;
    if (S.selection.has(u.id)) selectedChanged = true;
    img.width = u.width;
    img.height = u.height;
    img.palette = u.palette;
    img.weights = u.weights;
    img.colors = u.colors;
  }
  if (S.f.colors.size || S.f.shapes.size || S.f.minRes || S.sortKey === "dimensions") refresh();
  else {
    if (resized) layoutDirty = true;
    schedule("grid", "filterbar", "status");
    if (selectedChanged || !S.selection.size) schedule("inspector");
  }
});

// ---------------------------------------------------------------- start

async function start() {
  try {
    const snap = await Library.snapshot();
    S.folders = snap.folders;
    for (const img of snap.images) S.images.set(img.id, img);
  } catch (e) {
    if (isMyGo()) fail(e);
  }
  S.loaded = true;
  refresh();
}
start();
