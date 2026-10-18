package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// popup is the full-size image viewer. It loads its own bigger copy of the
// image rather than stretching the thumbnail, and in kitty mode uses its own
// image id so the thumbnail placement stays untouched.
type popup struct {
	msgIdx int
	att    beeper.Attachment
	src    string

	loading bool
	err     error

	// kitty
	id         uint32
	w, h       int
	cols, rows int

	// blocks
	img image.Image
}

const popupFirstID = 0x7E0000

type imageRef struct {
	msg int
	att beeper.Attachment
}

// chatImages lists every viewable image in the loaded messages, oldest first.
func (m *Model) chatImages() []imageRef {
	var out []imageRef
	for i, msg := range m.msgs {
		for _, a := range msg.Attachments {
			if thumbSource(a) != "" {
				out = append(out, imageRef{i, a})
			}
		}
	}
	return out
}

func messageImage(msg beeper.Message) (beeper.Attachment, bool) {
	for _, a := range msg.Attachments {
		if thumbSource(a) != "" {
			return a, true
		}
	}
	return beeper.Attachment{}, false
}

func (m *Model) viewImage(msgIdx int) tea.Cmd {
	if msgIdx < 0 || msgIdx >= len(m.msgs) {
		return nil
	}
	a, ok := messageImage(m.msgs[msgIdx])
	if !ok {
		m.setErr("no image in that message")
		return nil
	}
	return m.openPopup(msgIdx, a)
}

func (m *Model) openPopup(msgIdx int, a beeper.Attachment) tea.Cmd {
	cmd := m.closePopupImage()
	m.msgCur = msgIdx
	m.fixMsgScroll()
	p := &popup{msgIdx: msgIdx, att: a, src: thumbSource(a), loading: true}
	m.popup = p
	m.mode = modePopup

	mode := m.imageMode
	if mode == imagesOff {
		mode = imagesBlocks
	}
	if mode == imagesKitty {
		m.nextImageID++
		p.id = popupFirstID + m.nextImageID
	}
	load := m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		path, err := m.localPath(ctx, p.src)
		var (
			png  []byte
			img  image.Image
			w, h int
		)
		if err == nil {
			if mode == imagesKitty {
				png, w, h, err = shrinkPNG(ctx, path, "1600")
			} else {
				var b []byte
				if b, w, h, err = shrinkPNG(ctx, path, "400"); err == nil {
					img, _, err = image.Decode(bytes.NewReader(b))
				}
			}
		}
		return func(m *Model) tea.Cmd {
			if m.popup != p {
				return nil // closed or moved on while loading
			}
			p.loading, p.err = false, err
			if err != nil {
				return nil
			}
			p.w, p.h, p.img = w, h, img
			if mode == imagesKitty {
				return tea.Sequence(tea.Raw(kittyTransmit(p.id, png)), m.placePopup())
			}
			return nil
		}, nil
	})
	return tea.Batch(cmd, load)
}

// popupBox is the inner size available to the image.
func (m *Model) popupBox() (cols, rows int) {
	return max(10, m.w-8), max(3, m.bodyHeight()+2-6)
}

func (m *Model) placePopup() tea.Cmd {
	p := m.popup
	if p == nil || p.id == 0 || p.w == 0 {
		return nil
	}
	bw, bh := m.popupBox()
	c, r := fitCells(p.w, p.h, min(bw, len(diacritics)), min(bh, len(diacritics)), m.cellAspect)
	if c == p.cols && r == p.rows {
		return nil
	}
	p.cols, p.rows = c, r
	return tea.Raw(kittyPlace(p.id, c, r))
}

// closePopupImage frees the big copy in the terminal.
func (m *Model) closePopupImage() tea.Cmd {
	p := m.popup
	m.popup = nil
	if p == nil || p.id == 0 {
		return nil
	}
	return tea.Raw(kittyDelete(p.id))
}

func (m *Model) popupKey(key string) tea.Cmd {
	p := m.popup
	if p == nil {
		m.mode = modeNormal
		return nil
	}
	switch key {
	case "esc", "q", "enter", "v":
		m.mode = modeNormal
		return m.closePopupImage()
	case "d":
		return m.download(p.att)
	case "o":
		return m.openTarget(openTarget{label: firstNonEmpty(p.att.FileName, "image"), url: p.att.SrcURL, mime: p.att.MimeType, mxc: !strings.HasPrefix(p.att.SrcURL, "http")})
	case "n", "j", "l", "right", "down":
		return m.stepPopup(1)
	case "p", "k", "h", "left", "up":
		return m.stepPopup(-1)
	}
	return nil
}

func (m *Model) stepPopup(d int) tea.Cmd {
	imgs := m.chatImages()
	for i, r := range imgs {
		if r.msg == m.popup.msgIdx && r.att.SrcURL == m.popup.att.SrcURL {
			j := i + d
			if j < 0 || j >= len(imgs) {
				m.setStatus("no more images")
				return nil
			}
			return m.openPopup(imgs[j].msg, imgs[j].att)
		}
	}
	return nil
}

func (m *Model) renderPopup() string {
	p := m.popup
	title := "image"
	if p != nil {
		msg := m.msgs[min(p.msgIdx, len(m.msgs)-1)]
		who := firstNonEmpty(msg.SenderName, "?")
		if msg.IsSender {
			who = "you"
		}
		title = fmt.Sprintf("%s  %s", firstNonEmpty(p.att.FileName, "image"), stDim.Render(who+", "+shortTime(msg.Timestamp)))
		imgs := m.chatImages()
		for i, r := range imgs {
			if r.msg == p.msgIdx && r.att.SrcURL == p.att.SrcURL {
				title += stDim.Render(fmt.Sprintf("   %d of %d", i+1, len(imgs)))
			}
		}
	}
	bw, bh := m.popupBox()
	var body []string
	switch {
	case p == nil || p.loading:
		body = []string{stDim.Render("loading...")}
	case p.err != nil:
		body = []string{stErr.Render("couldn't load: " + p.err.Error())}
	case p.id != 0 && p.cols > 0:
		body = kittyLines(p.id, p.cols, p.rows)
	case p.img != nil:
		body = halfBlocks(p.img, bw, bh)
	}
	img := lipgloss.Place(bw, bh, lipgloss.Center, lipgloss.Center, strings.Join(body, "\n"))
	hint := stDim.Render("d download   o open   n/p next/prev   esc close")
	box := stOverlay.Width(m.w - 4).Render(stBold.Render(title) + "\n" + img + "\n" + hint)
	return lipgloss.Place(m.w, m.bodyHeight()+2, lipgloss.Center, lipgloss.Center, box)
}

// download copies an attachment into ~/Downloads without overwriting anything.
func (m *Model) download(a beeper.Attachment) tea.Cmd {
	if a.SrcURL == "" {
		m.setErr("nothing to download")
		return nil
	}
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		path, err := m.localPath(ctx, a.SrcURL)
		if err != nil {
			return nil, err
		}
		dest, err := saveToDownloads(path, a.FileName)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			if home, err := os.UserHomeDir(); err == nil {
				dest = strings.Replace(dest, home, "~", 1)
			}
			m.setStatus("saved " + dest)
			return m.notify("saved " + dest)
		}, nil
	})
}

func saveToDownloads(src, name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name = filepath.Base(name)
	if name == "" || name == "." || name == "/" {
		name = filepath.Base(src)
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	dest := filepath.Join(dir, name)
	for i := 1; ; i++ {
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			break
		}
		dest = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
	}
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	// O_EXCL so a file that appears between the check and here isn't clobbered
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dest)
		return "", err
	}
	return dest, out.Close()
}

// downloadMsg saves a message's attachment, asking which one if there are several.
func (m *Model) downloadMsg(msg beeper.Message) tea.Cmd {
	var atts []beeper.Attachment
	for _, a := range msg.Attachments {
		if a.SrcURL != "" {
			atts = append(atts, a)
		}
	}
	switch len(atts) {
	case 0:
		m.setErr("no attachment in that message")
		return nil
	case 1:
		return m.download(atts[0])
	}
	items := make([]pickItem, len(atts))
	for i, a := range atts {
		items[i] = pickItem{title: firstNonEmpty(a.FileName, a.Type), detail: a.MimeType, value: a}
	}
	m.openPicker("download which?", items, func(m *Model, it pickItem) tea.Cmd {
		return m.download(it.value.(beeper.Attachment))
	})
	return nil
}
