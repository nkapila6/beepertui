package ui

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// Two ways to draw images:
//
// kitty: the kitty graphics protocol with Unicode placeholders (Ghostty, kitty).
// The image is uploaded once with a virtual placement, then drawn by printing
// U+10EEEE cells whose foreground colour encodes the image id and whose
// diacritics encode row/column. They're ordinary text cells, so Bubble Tea's
// diffing renderer, scrolling and redraws all just work.
//
// blocks: half blocks, two pixels per cell, for every other truecolor terminal.
// Low res, but it works anywhere, including tmux.

type imageMode int

const (
	imagesOff imageMode = iota
	imagesBlocks
	imagesKitty
)

const (
	kittyMaxCols  = 60
	kittyMaxRows  = 20
	blockMaxCols  = 48
	blockMaxRows  = 12
	placeholder   = '\U0010EEEE'
	kittyFirstID  = 0x0BEE00 // ids double as 24-bit colours, keep them away from 0
	kittyMaxPixel = "800"
)

// first 80 entries of kitty's rowcolumn-diacritics.txt, which is plenty for our sizes
var diacritics = []rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F, 0x0346, 0x034A,
	0x034B, 0x034C, 0x0350, 0x0351, 0x0352, 0x0357, 0x035B, 0x0363, 0x0364, 0x0365,
	0x0366, 0x0367, 0x0368, 0x0369, 0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F,
	0x0483, 0x0484, 0x0485, 0x0486, 0x0487, 0x0592, 0x0593, 0x0594, 0x0595, 0x0597,
	0x0598, 0x0599, 0x059C, 0x059D, 0x059E, 0x059F, 0x05A0, 0x05A1, 0x05A8, 0x05A9,
	0x05AB, 0x05AC, 0x05AF, 0x05C4, 0x0610, 0x0611, 0x0612, 0x0613, 0x0614, 0x0615,
	0x0616, 0x0617, 0x0657, 0x0658, 0x0659, 0x065A, 0x065B, 0x065D, 0x065E, 0x06D6,
	0x06D7, 0x06D8, 0x06D9, 0x06DA, 0x06DB, 0x06DC, 0x06DF, 0x06E0, 0x06E1, 0x06E2,
}

// detectImageMode picks kitty graphics only where Unicode placeholders are known
// to work. tmux would need passthrough, so it gets blocks.
func detectImageMode() imageMode {
	switch os.Getenv("BEEPERTUI_IMAGES") {
	case "off":
		return imagesOff
	case "blocks":
		return imagesBlocks
	case "kitty":
		return imagesKitty
	}
	if os.Getenv("TMUX") != "" {
		return imagesBlocks
	}
	if os.Getenv("TERM_PROGRAM") == "ghostty" || os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return imagesKitty
	}
	return imagesBlocks
}

type thumb struct {
	loading bool
	failed  bool

	// blocks
	img   image.Image
	lines map[int][]string

	// kitty
	id         uint32
	w, h       int
	png        []byte // kept so the image can be re-sent after the terminal drops it
	sent       bool
	placedCols int
	placedRows int
}

func (t *thumb) ready() bool { return t != nil && (t.img != nil || t.sent) }

// thumbSource is the URL to show for an attachment: the image itself, or a
// video's poster frame.
func thumbSource(a beeper.Attachment) string {
	if a.Type == "video" {
		return a.PosterImg
	}
	if a.Type == "img" || strings.HasPrefix(a.MimeType, "image/") {
		return a.SrcURL
	}
	return ""
}

// loadThumbs kicks off fetches for any image in the loaded messages we haven't seen yet.
func (m *Model) loadThumbs() tea.Cmd {
	if !m.showImages || m.imageMode == imagesOff {
		return nil
	}
	var cmds []tea.Cmd
	for _, msg := range m.msgs {
		for _, a := range msg.Attachments {
			src := thumbSource(a)
			if src == "" || m.thumbs[src] != nil {
				continue
			}
			t := &thumb{loading: true}
			m.thumbs[src] = t
			mode := m.imageMode
			if mode == imagesKitty {
				m.nextImageID++
				t.id = kittyFirstID + m.nextImageID
			}
			id := t.id
			cmds = append(cmds, m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
				path, err := m.localPath(ctx, src)
				var (
					img  image.Image
					png  []byte
					w, h int
				)
				if err == nil {
					if mode == imagesKitty {
						png, w, h, err = shrinkPNG(ctx, path, kittyMaxPixel)
					} else {
						var b []byte
						if b, _, _, err = shrinkPNG(ctx, path, "160"); err == nil {
							img, _, err = image.Decode(bytes.NewReader(b))
						}
					}
				}
				return func(m *Model) tea.Cmd {
					t.loading = false
					if err != nil {
						t.failed = true
						return nil
					}
					var cmd tea.Cmd
					if mode == imagesKitty {
						t.w, t.h, t.sent, t.png = w, h, true, png
						cmd = tea.Raw(kittyTransmit(id, png))
					} else {
						t.img = img
					}
					m.fixMsgScroll()
					return tea.Sequence(cmd, m.placeImages())
				}, nil
			}))
		}
	}
	return tea.Batch(cmds...)
}

// shrinkPNG uses sips (ships with macOS, reads HEIC/WebP/TIFF/...) to make a
// PNG no bigger than maxPx on its long side.
func shrinkPNG(ctx context.Context, path, maxPx string) ([]byte, int, int, error) {
	tmp, err := os.CreateTemp("", "beepertui-thumb-*.png")
	if err != nil {
		return nil, 0, 0, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	src := path
	if exec.CommandContext(ctx, "sips", "-s", "format", "png", "-Z", maxPx, path, "--out", tmp.Name()).Run() == nil {
		src = tmp.Name()
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return nil, 0, 0, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, 0, 0, err
	}
	return b, cfg.Width, cfg.Height, nil
}

func (m *Model) localPath(ctx context.Context, src string) (string, error) {
	switch {
	case strings.HasPrefix(src, "mxc://"), strings.HasPrefix(src, "localmxc://"):
		local, err := m.c.Download(ctx, src)
		if err != nil {
			return "", err
		}
		src = local
	case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
		return "", fmt.Errorf("remote image")
	}
	if strings.HasPrefix(src, "file://") {
		u, err := url.Parse(src)
		if err != nil {
			return "", err
		}
		src = u.Path
	}
	return src, nil
}

// kitty

func kittyTransmit(id uint32, png []byte) string {
	data := base64.StdEncoding.EncodeToString(png)
	var b strings.Builder
	const chunk = 4096
	for i := 0; i < len(data); i += chunk {
		end := min(i+chunk, len(data))
		more := 0
		if end < len(data) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Ga=t,f=100,t=d,i=%d,q=2,m=%d;%s\x1b\\", id, more, data[i:end])
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, data[i:end])
		}
	}
	return b.String()
}

// fitCells sizes a w x h pixel image into at most maxCols x maxRows cells,
// keeping its shape. aspect is cell height / width.
func fitCells(w, h, maxCols, maxRows int, aspect float64) (cols, rows int) {
	if w == 0 || h == 0 || maxCols <= 0 || maxRows <= 0 {
		return 0, 0
	}
	cols = maxCols
	rows = max(1, int(float64(cols)*float64(h)/float64(w)/aspect+0.5))
	if rows > maxRows {
		rows = maxRows
		cols = max(1, int(float64(rows)*aspect*float64(w)/float64(h)+0.5))
	}
	return min(cols, maxCols), rows
}

// kittySize fits a thumbnail into the text width.
func kittySize(t *thumb, width int, aspect float64) (cols, rows int) {
	return fitCells(t.w, t.h, min(width, kittyMaxCols, len(diacritics)), kittyMaxRows, aspect)
}

// placeImages (re)creates virtual placements whose size no longer matches the
// pane, e.g. after a resize. The placement is what tells the terminal how many
// cells the placeholders span.
func (m *Model) placeImages() tea.Cmd {
	if m.imageMode != imagesKitty {
		return nil
	}
	w := m.msgTextWidth()
	var b strings.Builder
	for _, t := range m.thumbs {
		if !t.sent {
			continue
		}
		c, r := kittySize(t, w, m.cellAspect)
		if c == t.placedCols && r == t.placedRows {
			continue
		}
		b.WriteString(kittyPlace(t.id, c, r))
		t.placedCols, t.placedRows = c, r
	}
	if b.Len() == 0 {
		return nil
	}
	return tea.Raw(b.String())
}

func kittyPlace(id uint32, cols, rows int) string {
	return fmt.Sprintf("\x1b_Ga=p,U=1,i=%d,p=1,c=%d,r=%d,C=1,q=2\x1b\\", id, cols, rows)
}

func kittyDelete(id uint32) string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
}

// kittyLines prints the placeholder cells for a placed image.
func kittyLines(id uint32, cols, rows int) []string {
	st := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("#%06x", id&0xFFFFFF)))
	lines := make([]string, rows)
	for r := range rows {
		var sb strings.Builder
		for c := range cols {
			sb.WriteRune(placeholder)
			sb.WriteRune(diacritics[r])
			sb.WriteRune(diacritics[c])
		}
		lines[r] = st.Render(sb.String())
	}
	return lines
}

// thumbLines renders a loaded image to fit width columns.
func (m *Model) thumbLines(src string, width int) []string {
	t := m.thumbs[src]
	if !t.ready() {
		return nil
	}
	if t.sent {
		if t.placedCols == 0 {
			return nil
		}
		return kittyLines(t.id, t.placedCols, t.placedRows)
	}
	if l, ok := t.lines[width]; ok {
		return l
	}
	lines := halfBlocks(t.img, min(width, blockMaxCols), blockMaxRows)
	if t.lines == nil {
		t.lines = map[int][]string{}
	}
	t.lines[width] = lines
	return lines
}

func halfBlocks(img image.Image, maxCols, maxRows int) []string {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return nil
	}
	// a half block cell is two pixels tall, so the cell aspect is effectively 1:1 in pixels
	cols, rows := fitCells(b.Dx(), b.Dy(), min(maxCols, b.Dx()), maxRows*2, 1)
	rows = max(1, (rows+1)/2)
	px := func(x, y int) color.Color {
		// box sample the source area that maps to this pixel
		x0, x1 := b.Min.X+x*b.Dx()/cols, b.Min.X+(x+1)*b.Dx()/cols
		y0, y1 := b.Min.Y+y*b.Dy()/(rows*2), b.Min.Y+(y+1)*b.Dy()/(rows*2)
		var r, g, bl, n uint32
		for yy := y0; yy < max(y1, y0+1); yy++ {
			for xx := x0; xx < max(x1, x0+1); xx++ {
				cr, cg, cb, _ := img.At(xx, yy).RGBA()
				r, g, bl, n = r+cr>>8, g+cg>>8, bl+cb>>8, n+1
			}
		}
		return color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), 255}
	}
	lines := make([]string, rows)
	for y := range rows {
		var sb strings.Builder
		for x := range cols {
			sb.WriteString(lipgloss.NewStyle().Foreground(px(x, y*2)).Background(px(x, y*2+1)).Render("▀"))
		}
		lines[y] = sb.String()
	}
	return lines
}

type resizedMsg struct{ seq int }

// afterResize waits for resizing to settle, then re-sends every image. Some
// terminals (Ghostty especially) drop kitty placements on resize, and the
// placeholder cells then draw nothing.
func (m *Model) afterResize() tea.Cmd {
	if m.imageMode != imagesKitty {
		return nil
	}
	m.resizeSeq++
	seq := m.resizeSeq
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return resizedMsg{seq} })
}

func (m *Model) resendImages() tea.Cmd {
	var b strings.Builder
	w := m.msgTextWidth()
	for _, t := range m.thumbs {
		if !t.sent || len(t.png) == 0 {
			continue
		}
		c, r := kittySize(t, w, m.cellAspect)
		b.WriteString(kittyTransmit(t.id, t.png))
		b.WriteString(kittyPlace(t.id, c, r))
		t.placedCols, t.placedRows = c, r
	}
	if p := m.popup; p != nil && p.id != 0 && len(p.png) > 0 {
		b.WriteString(kittyTransmit(p.id, p.png))
		p.cols, p.rows = 0, 0 // force placePopup to place again
	}
	b.WriteString(m.avatarResend())
	if b.Len() == 0 {
		return nil
	}
	return tea.Sequence(tea.Raw(b.String()), m.placePopup())
}
