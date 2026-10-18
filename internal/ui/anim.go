package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"net/http"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Animated GIFs. The standard library splits a GIF into frames; we composite
// them (frames often only redraw the part that changed), shrink them, and
// play them ourselves:
//
//	kitty mode  every frame is uploaded once as its own image, and each tick
//	            only changes which image id the placeholder cells name. That
//	            works in Ghostty and tuios too, which don't run kitty's own
//	            animation protocol.
//	blocks      each frame is drawn as half blocks, cached per width.

const (
	maxAnimFrames = 60
	animTick      = 40 * time.Millisecond
)

type animFrame struct {
	img   image.Image // blocks mode
	png   []byte      // kitty mode
	id    uint32
	delay time.Duration
}

// media is a loaded picture: a still, or a still plus frames when animated.
type media struct {
	png    []byte
	img    image.Image
	w, h   int
	frames []animFrame
}

// loadMedia reads an image file for display. GIFs with more than one frame
// come back with frames; anything else goes through shrinkPNG as before.
func loadMedia(ctx context.Context, path string, maxPx int, kitty bool) (media, error) {
	if isGIF(path) {
		if m, err := decodeGIF(path, maxPx, kitty); err == nil && len(m.frames) > 1 {
			return m, nil
		}
	}
	b, w, h, err := shrinkPNG(ctx, path, fmt.Sprint(maxPx))
	if err != nil {
		return media{}, err
	}
	m := media{png: b, w: w, h: h}
	if !kitty {
		if m.img, _, err = image.Decode(bytes.NewReader(b)); err != nil {
			return media{}, err
		}
	}
	return m, nil
}

func isGIF(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	return http.DetectContentType(head[:n]) == "image/gif"
}

func decodeGIF(path string, maxPx int, kitty bool) (media, error) {
	f, err := os.Open(path)
	if err != nil {
		return media{}, err
	}
	defer f.Close()
	g, err := gif.DecodeAll(f)
	if err != nil || len(g.Image) == 0 {
		return media{}, fmt.Errorf("gif: %v", err)
	}

	// long gifs keep every nth frame, with the skipped delays folded in
	step := (len(g.Image) + maxAnimFrames - 1) / maxAnimFrames
	cw, ch := g.Config.Width, g.Config.Height
	if cw == 0 || ch == 0 {
		b := g.Image[0].Bounds()
		cw, ch = b.Dx(), b.Dy()
	}
	canvas := image.NewRGBA(image.Rect(0, 0, cw, ch))
	var out media
	var pending time.Duration
	for i, frame := range g.Image {
		var before *image.RGBA
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			before = image.NewRGBA(canvas.Bounds())
			copy(before.Pix, canvas.Pix)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)

		delay := 100 * time.Millisecond
		if i < len(g.Delay) && g.Delay[i] > 1 {
			delay = time.Duration(g.Delay[i]) * 10 * time.Millisecond
		}
		pending += delay
		if i%step == 0 {
			small := scaleDown(canvas, maxPx)
			fr := animFrame{delay: pending}
			pending = 0
			if kitty {
				var buf bytes.Buffer
				if err := png.Encode(&buf, small); err != nil {
					return media{}, err
				}
				fr.png = buf.Bytes()
			} else {
				fr.img = small
			}
			out.frames = append(out.frames, fr)
			if len(out.frames) == 1 {
				out.w, out.h = small.Bounds().Dx(), small.Bounds().Dy()
				out.png, out.img = fr.png, fr.img
			}
		} else if len(out.frames) > 0 {
			out.frames[len(out.frames)-1].delay += delay
			pending = 0
		}

		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = before
		}
	}
	return out, nil
}

// scaleDown box-samples an image so its long side is at most maxPx.
func scaleDown(src *image.RGBA, maxPx int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxPx && h <= maxPx {
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		copy(dst.Pix, src.Pix)
		return dst
	}
	nw, nh := maxPx, h*maxPx/w
	if h > w {
		nw, nh = w*maxPx/h, maxPx
	}
	nw, nh = max(1, nw), max(1, nh)
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := range nh {
		y0, y1 := y*h/nh, max((y+1)*h/nh, y*h/nh+1)
		for x := range nw {
			x0, x1 := x*w/nw, max((x+1)*w/nw, x*w/nw+1)
			var r, g, bl, a, n uint32
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := src.RGBAAt(xx, yy)
					r, g, bl, a, n = r+uint32(c.R), g+uint32(c.G), bl+uint32(c.B), a+uint32(c.A), n+1
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	return dst
}

// the playing state lives on thumb (frames, cur, next) so message thumbnails,
// the image viewer and picker previews all animate the same way

func (t *thumb) animated() bool { return t != nil && len(t.frames) > 1 }

// frameID is the kitty image id to draw now.
func (t *thumb) frameID() uint32 {
	if t.animated() {
		return t.frames[t.frame].id
	}
	return t.id
}

// setMedia fills a thumb from loaded media, giving every frame a kitty id.
// It returns the escapes that upload the frames, for kitty mode.
func (m *Model) setMedia(t *thumb, md media, kitty bool) string {
	t.w, t.h, t.img = md.w, md.h, md.img
	t.frames, t.frame, t.next = md.frames, 0, time.Time{}
	if !kitty {
		return ""
	}
	t.png, t.sent = md.png, true
	var b strings.Builder
	b.WriteString(kittyTransmit(t.id, md.png))
	for i := range t.frames {
		if i == 0 {
			t.frames[i].id = t.id
			continue
		}
		m.nextImageID++
		t.frames[i].id = t.id + 0x1000 + m.nextImageID
		b.WriteString(kittyTransmit(t.frames[i].id, t.frames[i].png))
	}
	return b.String()
}

// placeAll places every frame of a thumb at the same size.
func (t *thumb) placeAll(cols, rows int) string {
	var b strings.Builder
	b.WriteString(kittyPlace(t.id, cols, rows))
	for i, f := range t.frames {
		if i > 0 {
			b.WriteString(kittyPlace(f.id, cols, rows))
		}
	}
	return b.String()
}

func (t *thumb) resend() string {
	var b strings.Builder
	b.WriteString(kittyTransmit(t.id, t.png))
	for i, f := range t.frames {
		if i > 0 && len(f.png) > 0 {
			b.WriteString(kittyTransmit(f.id, f.png))
		}
	}
	return b.String()
}

func (t *thumb) deleteAll() string {
	var b strings.Builder
	b.WriteString(kittyDelete(t.id))
	for i, f := range t.frames {
		if i > 0 {
			b.WriteString(kittyDelete(f.id))
		}
	}
	return b.String()
}

// blockFrame is the image to draw in blocks mode right now.
func (t *thumb) blockFrame() image.Image {
	if t.animated() && t.frames[t.frame].img != nil {
		return t.frames[t.frame].img
	}
	return t.img
}

type animMsg struct{}

// startAnim begins ticking if anything animated is loaded and it isn't already running.
func (m *Model) startAnim() tea.Cmd {
	if m.animating || !m.anyAnimated() {
		return nil
	}
	m.animating = true
	return tea.Tick(animTick, func(time.Time) tea.Msg { return animMsg{} })
}

func (m *Model) allThumbs() []*thumb {
	var ts []*thumb
	for _, t := range m.thumbs {
		ts = append(ts, t)
	}
	if m.popup != nil && m.popup.t != nil {
		ts = append(ts, m.popup.t)
	}
	if m.gif != nil {
		for _, t := range m.gif.preview {
			ts = append(ts, t)
		}
	}
	return ts
}

func (m *Model) anyAnimated() bool {
	for _, t := range m.allThumbs() {
		if t.animated() {
			return true
		}
	}
	return false
}

// stepAnim advances every animated thumb whose frame is due.
func (m *Model) stepAnim() tea.Cmd {
	if !m.anyAnimated() || !m.showImages {
		m.animating = false
		return nil
	}
	now := time.Now()
	for _, t := range m.allThumbs() {
		if !t.animated() {
			continue
		}
		if t.next.IsZero() {
			t.next = now.Add(t.frames[t.frame].delay)
			continue
		}
		if now.After(t.next) {
			t.frame = (t.frame + 1) % len(t.frames)
			t.next = now.Add(t.frames[t.frame].delay)
		}
	}
	return tea.Tick(animTick, func(time.Time) tea.Msg { return animMsg{} })
}
