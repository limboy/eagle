package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// Kind is the family of an image file, as the Type filter shows it.
type Kind string

const (
	KindJPG  Kind = "JPG"
	KindPNG  Kind = "PNG"
	KindHEIC Kind = "HEIC"
	KindWEBP Kind = "WEBP"
	KindGIF  Kind = "GIF"
	KindTIFF Kind = "TIFF"
	KindBMP  Kind = "BMP"
	KindAVIF Kind = "AVIF"
	KindRAW  Kind = "RAW"
)

var kindByExt = map[string]Kind{
	".jpg": KindJPG, ".jpeg": KindJPG, ".jpe": KindJPG,
	".png":  KindPNG,
	".heic": KindHEIC, ".heif": KindHEIC,
	".webp": KindWEBP,
	".gif":  KindGIF,
	".tif":  KindTIFF, ".tiff": KindTIFF,
	".bmp":  KindBMP,
	".avif": KindAVIF,
	".arw":  KindRAW, ".cr2": KindRAW, ".cr3": KindRAW, ".nef": KindRAW, ".raf": KindRAW,
	".dng": KindRAW, ".orf": KindRAW, ".rw2": KindRAW, ".pef": KindRAW, ".srw": KindRAW,
}

// webSafe kinds are shown by the page as they are; the others get a JPEG
// preview for the viewer.
var webSafe = map[Kind]bool{KindJPG: true, KindPNG: true, KindGIF: true, KindWEBP: true}

// goDecodable kinds are read by Go's image packages.
var goDecodable = map[Kind]bool{KindJPG: true, KindPNG: true, KindGIF: true, KindWEBP: true, KindTIFF: true, KindBMP: true}

const (
	thumbSize   = 720  // longest edge of grid thumbnails, in pixels
	previewSize = 3200 // longest edge of viewer previews of non-web formats
)

// readDimensions returns the displayed size of the image at path: its
// pixel size, with width and height swapped when its orientation turns it.
func readDimensions(path string, kind Kind) (int, int, error) {
	if goDecodable[kind] {
		f, err := os.Open(path)
		if err != nil {
			return 0, 0, err
		}
		defer f.Close()
		br := bufio.NewReader(f)
		orientation := 1
		if kind == KindJPG {
			head, _ := br.Peek(128 * 1024)
			orientation = jpegOrientation(head)
		}
		cfg, _, err := image.DecodeConfig(br)
		if err == nil {
			if orientation >= 5 && orientation <= 8 {
				return cfg.Height, cfg.Width, nil
			}
			return cfg.Width, cfg.Height, nil
		}
	}
	return sipsDimensions(path)
}

// sipsDimensions asks macOS's ImageIO, through sips, for the size of formats
// Go can't read, such as HEIC and camera RAW.
func sipsDimensions(path string) (int, int, error) {
	if runtime.GOOS != "darwin" {
		return 0, 0, errors.New("unsupported image format")
	}
	out, err := exec.Command("sips", "-g", "pixelWidth", "-g", "pixelHeight", "-g", "orientation", path).Output()
	if err != nil {
		return 0, 0, err
	}
	var w, h int
	orientation := 1
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "pixelWidth":
			w, _ = strconv.Atoi(v)
		case "pixelHeight":
			h, _ = strconv.Atoi(v)
		case "orientation":
			orientation, _ = strconv.Atoi(v)
		}
	}
	if w == 0 || h == 0 {
		return 0, 0, errors.New("no image size")
	}
	if orientation >= 5 && orientation <= 8 {
		w, h = h, w
	}
	return w, h, nil
}

// jpegOrientation reads the EXIF orientation from the start of a JPEG file,
// 1 (upright) when it has none.
func jpegOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		if marker == 0xDA || marker == 0xD9 {
			return 1
		}
		size := int(binary.BigEndian.Uint16(b[i+2:]))
		seg := i + 4
		if marker == 0xE1 && seg+size-2 <= len(b) && bytes.HasPrefix(b[seg:], []byte("Exif\x00\x00")) {
			return tiffOrientation(b[seg+6 : seg+size-2])
		}
		i = seg + size - 2
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[ifd:]))
	for k := 0; k < n; k++ {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			o := int(bo.Uint16(t[e+8:]))
			if o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}

// makeJPEG writes a JPEG copy of src to dst, its longest edge at most size
// pixels. macOS's sips reads every format ImageIO knows, HEIC and RAW
// included, and keeps the orientation; elsewhere, and when sips fails, Go
// decodes the formats it knows.
func makeJPEG(src, dst string, kind Kind, size int) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp.jpg"
	defer os.Remove(tmp)
	if runtime.GOOS == "darwin" {
		args := []string{"-s", "format", "jpeg", "-s", "formatOptions", "82"}
		// -Z scales small images up as well as large ones down.
		if w, h, err := sipsDimensions(src); err != nil || max(w, h) > size {
			args = append(args, "-Z", strconv.Itoa(size))
		}
		cmd := exec.Command("sips", append(args, src, "--out", tmp)...)
		if err := cmd.Run(); err == nil {
			if st, err := os.Stat(tmp); err == nil && st.Size() > 0 {
				return os.Rename(tmp, dst)
			}
		}
	}
	if !goDecodable[kind] {
		return fmt.Errorf("can't read %s images here", kind)
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > size || h > size {
		if w >= h {
			h = int(math.Round(float64(h) * float64(size) / float64(w)))
			w = size
		} else {
			w = int(math.Round(float64(w) * float64(size) / float64(h)))
			h = size
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.ApproxBiLinear.Scale(out, out.Bounds(), img, b, draw.Src, nil)
	o, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := jpeg.Encode(o, out, &jpeg.Options{Quality: 82}); err != nil {
		o.Close()
		return err
	}
	if err := o.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// Swatch is one of an image's dominant colors and the share of it it covers.
type Swatch struct {
	Hex    string
	Weight float64
}

// extractPalette finds up to five distinct dominant colors of an image,
// read from a small JPEG such as its thumbnail.
func extractPalette(r io.Reader) ([]Swatch, error) {
	img, err := jpeg.Decode(r)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	step := max(1, max(b.Dx(), b.Dy())/96)
	type acc struct{ r, g, b, n float64 }
	buckets := map[int]*acc{}
	total := 0.0
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bl, _ := img.At(x, y).RGBA()
			r8, g8, b8 := r>>8, g>>8, bl>>8
			k := int(r8>>5)<<6 | int(g8>>5)<<3 | int(b8>>5)
			a := buckets[k]
			if a == nil {
				a = &acc{}
				buckets[k] = a
			}
			a.r += float64(r8)
			a.g += float64(g8)
			a.b += float64(b8)
			a.n++
			total++
		}
	}
	list := make([]*acc, 0, len(buckets))
	for _, a := range buckets {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	var out []Swatch
	var rgb [][3]float64
	for _, a := range list {
		if len(out) == 5 {
			break
		}
		c := [3]float64{a.r / a.n, a.g / a.n, a.b / a.n}
		near := false
		for _, p := range rgb {
			d := (p[0]-c[0])*(p[0]-c[0]) + (p[1]-c[1])*(p[1]-c[1]) + (p[2]-c[2])*(p[2]-c[2])
			if d < 40*40 {
				near = true
				break
			}
		}
		if near {
			continue
		}
		rgb = append(rgb, c)
		out = append(out, Swatch{
			Hex:    fmt.Sprintf("#%02x%02x%02x", int(c[0]), int(c[1]), int(c[2])),
			Weight: math.Round(a.n/total*1000) / 1000,
		})
	}
	return out, nil
}

// colorFamilies names the color families an image belongs to, for the
// color filter: those of its first color and of each color covering at
// least 3.5% of it.
func colorFamilies(p []Swatch) []string {
	seen := map[string]bool{}
	var out []string
	for i, s := range p {
		if i > 0 && s.Weight < 0.035 {
			continue
		}
		f := colorFamily(s.Hex)
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

func colorFamily(hex string) string {
	n, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	r := float64((n>>16)&255) / 255
	g := float64((n>>8)&255) / 255
	b := float64(n&255) / 255
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l := (mx + mn) / 2
	d := mx - mn
	h, s := 0.0, 0.0
	if d > 0 {
		s = d / (1 - math.Abs(2*l-1))
		switch mx {
		case r:
			h = math.Mod((g-b)/d, 6)
		case g:
			h = (b-r)/d + 2
		default:
			h = (r-g)/d + 4
		}
		h *= 60
		if h < 0 {
			h += 360
		}
	}
	// Neutrals are told by chroma (d), not HSL saturation, which grows
	// without bound near white and black: cream would otherwise be orange.
	switch {
	case l < 0.13:
		return "black"
	case l > 0.86 && d < 0.2:
		return "white"
	case d < 0.1 || s < 0.12:
		if l > 0.78 {
			return "white"
		}
		if l < 0.24 {
			return "black"
		}
		return "gray"
	case (h < 45 || h >= 340) && l < 0.46 && s < 0.65:
		return "brown"
	case h >= 345 || h < 14:
		return "red"
	case h < 42:
		return "orange"
	case h < 66:
		return "yellow"
	case h < 160:
		return "green"
	case h < 195:
		return "teal"
	case h < 252:
		return "blue"
	case h < 292:
		return "purple"
	default:
		return "pink"
	}
}
