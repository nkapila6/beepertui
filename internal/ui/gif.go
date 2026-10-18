package ui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// GIF search through KLIPY or GIPHY (Tenor's API shut down in 2026). Both
// need a free key, BEEPERTUI_KLIPY_KEY or BEEPERTUI_GIPHY_KEY. As both
// providers require, requests come straight from this client, media loads from
// their URLs, and the picker credits whoever served the results.

// BEEPERTUI_KLIPY_URL points at a stand-in API, for demo/server
var klipyBase = firstNonEmpty(os.Getenv("BEEPERTUI_KLIPY_URL"), "https://api.klipy.com/api/v1/")

type gifResult struct {
	slug, title   string
	source        string // klipy, giphy, memegen, nekos, kitchen, cataas, xkcd, status, reddit
	kind          string // what the status line calls it: gif, meme, sticker...
	attType       string // overrides Beeper's attachment type, e.g. sticker
	header        string // set on the All tab's section headers, which aren't selectable
	meme          *memeTemplate
	sendURL       string // what gets uploaded to the chat (may be a ref like xkcd:N)
	previewURL    string // a small still for the picker
	width, height int

	// results with a caption step (memes, cats): the final URL for a caption,
	// a quick preview URL for it, and the help shown while typing
	captionURL     func(text string) string
	captionPreview func(text string) string
	captionHelp    func(width int) []string
}

type gifPicker struct {
	query   string
	results []gifResult
	cur     int
	loading bool
	err     error
	seq     int // ignores responses to stale queries
	preview map[string]*thumb
	tab     int    // index into mediaTabs
	gifProv string // klipy, giphy or "" (no key)

	// caption step for memes: the picked template, the live preview url, and
	// the search to go back to
	captioning *gifResult
	capURL     string
	savedQuery string
}

type gifSearchMsg struct{ seq int }

func klipyKey() string { return strings.TrimSpace(os.Getenv("BEEPERTUI_KLIPY_KEY")) }
func giphyKey() string { return strings.TrimSpace(os.Getenv("BEEPERTUI_GIPHY_KEY")) }

// gifProvider picks klipy or giphy from the setting and whichever key is set.
func (m *Model) gifProvider() string {
	want := m.cfg.String("gif_provider")
	switch {
	case want == "klipy" && klipyKey() != "":
		return "klipy"
	case want == "giphy" && giphyKey() != "":
		return "giphy"
	case want == "auto" && klipyKey() != "":
		return "klipy"
	case want == "auto" && giphyKey() != "":
		return "giphy"
	}
	return ""
}

// customerID is a stable, anonymous id per user and machine, which KLIPY uses for "recent".
func customerID() string {
	name := "beepertui"
	if u, err := user.Current(); err == nil {
		name += u.Username
	}
	if h, err := os.Hostname(); err == nil {
		name += h
	}
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:8])
}

// openGifPicker is ctrl+g: the last tab used, else GIFs with a key or All without.
func (m *Model) openGifPicker(query string) tea.Cmd {
	tab := m.mediaTab
	if tab == "" {
		tab = "all"
		if m.gifProvider() != "" {
			tab = "gifs"
		}
		if m.cfg.String("gif_provider") == "memegen" {
			tab = "memes"
		}
	}
	return m.openPickerFor(tab, query)
}

// openPickerFor opens the media picker on a tab (all, gifs, memes, ...).
func (m *Model) openPickerFor(tab, query string) tea.Cmd {
	if m.cur == nil {
		m.setErr("open a chat first")
		return nil
	}
	m.gif = &gifPicker{query: query, preview: map[string]*thumb{}, tab: tabIndex(tab), gifProv: m.gifProvider()}
	m.mediaTab = tab
	m.mode = modeGif
	m.line.Prompt = "search> "
	m.line.SetValue(query)
	m.line.CursorEnd()
	return tea.Batch(m.line.Focus(), m.searchGifs())
}

// switchTab moves the picker to another tab, keeping the search text.
func (m *Model) switchTab(d int) tea.Cmd {
	g := m.gif
	g.tab = (g.tab + d + len(mediaTabs)) % len(mediaTabs)
	m.mediaTab = mediaTabs[g.tab].id
	g.results, g.cur, g.err = nil, 0, nil
	return m.searchGifs()
}

func (m *Model) searchGifs() tea.Cmd {
	g := m.gif
	g.seq++
	seq, q, tab, prov := g.seq, strings.TrimSpace(m.line.Value()), mediaTabs[g.tab].id, g.gifProv
	g.loading = true
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		var res []gifResult
		var err error
		if tab == "all" {
			res, err = m.allSearch(ctx, prov, q)
		} else {
			res, err = m.sourceSearch(ctx, tab, prov, q, 30)
		}
		return func(m *Model) tea.Cmd {
			if m.gif == nil || m.gif.seq != seq {
				return nil
			}
			m.gif.loading, m.gif.err = false, err
			m.gif.results, m.gif.cur = res, 0
			m.gif.skipHeaders(1)
			return tea.Batch(m.loadGifPreview(), m.placeGifPreview())
		}, nil
	})
}

// skipHeaders moves the cursor off section headers, in direction d.
func (g *gifPicker) skipHeaders(d int) {
	for range len(g.results) {
		if g.cur < len(g.results) && g.results[g.cur].header == "" {
			return
		}
		g.cur = (g.cur + d + len(g.results)) % len(g.results)
	}
}

func klipySearch(ctx context.Context, q string) ([]gifResult, error) {
	v := url.Values{"per_page": {"24"}, "customer_id": {customerID()}, "content_filter": {"medium"}, "format_filter": {"gif,jpg"}}
	endpoint := "gifs/trending"
	if q != "" {
		endpoint = "gifs/search"
		v.Set("q", q)
	}
	u := klipyBase + url.PathEscape(klipyKey()) + "/" + endpoint + "?" + v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("klipy: %s", resp.Status)
	}
	type media struct {
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	type size struct {
		GIF *media `json:"gif"`
		JPG *media `json:"jpg"`
	}
	var out struct {
		Data struct {
			Data []struct {
				Slug  string          `json:"slug"`
				Title string          `json:"title"`
				File  map[string]size `json:"file"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	pick := func(f map[string]size, want func(size) *media, order ...string) *media {
		for _, k := range order {
			if s, ok := f[k]; ok && want(s) != nil && want(s).URL != "" {
				return want(s)
			}
		}
		return nil
	}
	var res []gifResult
	for _, d := range out.Data.Data {
		send := pick(d.File, func(s size) *media { return s.GIF }, "md", "hd", "sm")
		still := pick(d.File, func(s size) *media { return s.JPG }, "sm", "md", "hd")
		if send == nil {
			continue
		}
		r := gifResult{source: "klipy", kind: "gif", slug: d.Slug, title: firstNonEmpty(d.Title, d.Slug), sendURL: send.URL, width: send.Width, height: send.Height}
		if still != nil {
			r.previewURL = still.URL
		}
		res = append(res, r)
	}
	return res, nil
}

func giphySearch(ctx context.Context, q string) ([]gifResult, error) {
	v := url.Values{"api_key": {giphyKey()}, "limit": {"24"}, "rating": {"pg-13"}, "bundle": {"messaging_non_clips"}}
	endpoint := "trending"
	if q != "" {
		endpoint = "search"
		v.Set("q", q)
	}
	base := firstNonEmpty(os.Getenv("BEEPERTUI_GIPHY_URL"), "https://api.giphy.com/v1/gifs/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+endpoint+"?"+v.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("giphy: %s", resp.Status)
	}
	type rendition struct {
		URL    string `json:"url"`
		Width  string `json:"width"`
		Height string `json:"height"`
	}
	var out struct {
		Data []struct {
			Slug   string `json:"slug"`
			Title  string `json:"title"`
			Images struct {
				FixedHeight      rendition `json:"fixed_height"`
				Downsized        rendition `json:"downsized"`
				FixedHeightStill rendition `json:"fixed_height_still"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	var res []gifResult
	for _, d := range out.Data {
		send := d.Images.Downsized
		if send.URL == "" {
			send = d.Images.FixedHeight
		}
		if send.URL == "" {
			continue
		}
		var w, h int
		fmt.Sscan(send.Width, &w)
		fmt.Sscan(send.Height, &h)
		res = append(res, gifResult{source: "giphy", kind: "gif", slug: d.Slug, title: firstNonEmpty(d.Title, d.Slug), sendURL: send.URL, previewURL: d.Images.FixedHeightStill.URL, width: w, height: h})
	}
	return res, nil
}

// current is the preview on show: the captioned meme while captioning, else
// the highlighted result. The key doubles as the preview cache key.
func (g *gifPicker) current() (key, url string) {
	if g.captioning != nil {
		if g.capURL != "" {
			return g.capURL, g.capURL
		}
		return g.captioning.slug, g.captioning.previewURL
	}
	if g.cur < len(g.results) && g.results[g.cur].header == "" {
		r := g.results[g.cur]
		return r.source + ":" + r.slug, r.previewURL
	}
	return "", ""
}

// loadGifPreview fetches the highlighted result's still for the preview.
func (m *Model) loadGifPreview() tea.Cmd {
	g := m.gif
	if g == nil || (g.captioning == nil && g.cur >= len(g.results)) {
		return nil
	}
	key, previewURL := g.current()
	if previewURL == "" || g.preview[key] != nil {
		return nil
	}
	t := &thumb{loading: true}
	g.preview[key] = t
	kitty := m.imageMode == imagesKitty
	if kitty {
		m.nextImageID++
		t.id = kittyFirstID + 0x10000 + m.nextImageID
	}
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		u, err := resolveURL(ctx, previewURL)
		var path string
		if err == nil {
			path, err = fetchTemp(ctx, u, "beepertui-gifprev-*.img")
		}
		var png []byte
		var w, h int
		var img image.Image
		if err == nil {
			defer os.Remove(path)
			png, w, h, err = shrinkPNG(ctx, path, "400")
			if err == nil && !kitty {
				img, _, err = image.Decode(bytes.NewReader(png))
			}
		}
		return func(m *Model) tea.Cmd {
			t.loading = false
			if err != nil {
				t.failed = true
				return nil
			}
			t.w, t.h, t.img = w, h, img
			if kitty {
				t.png, t.sent = png, true
				return tea.Sequence(tea.Raw(kittyTransmit(t.id, png)), m.placeGifPreview())
			}
			return nil
		}, nil
	})
}

func fetchTemp(ctx context.Context, u, pattern string) (string, error) {
	// https only, apart from a local stand-in server
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://127.0.0.1:") {
		return "", errors.New("refusing non-https media url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("download: %s", resp.Status)
	}
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	// gifs are small, but cap it anyway
	if _, err := io.Copy(f, io.LimitReader(resp.Body, 25<<20)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), f.Close()
}

func (m *Model) gifKey(k tea.KeyPressMsg) tea.Cmd {
	g := m.gif
	if g.captioning != nil {
		return m.captionKey(k)
	}
	switch k.String() {
	case "esc":
		return m.closeGif()
	case "tab", "ctrl+right", "alt+right":
		return m.switchTab(1)
	case "shift+tab", "ctrl+left", "alt+left":
		return m.switchTab(-1)
	case "up", "ctrl+p", "ctrl+k":
		if len(g.results) > 0 {
			g.cur = (g.cur - 1 + len(g.results)) % len(g.results)
			g.skipHeaders(-1)
		}
		return tea.Batch(m.loadGifPreview(), m.placeGifPreview())
	case "down", "ctrl+n", "ctrl+j":
		if len(g.results) > 0 {
			g.cur = (g.cur + 1) % len(g.results)
			g.skipHeaders(1)
		}
		return tea.Batch(m.loadGifPreview(), m.placeGifPreview())
	case "enter":
		if g.cur >= len(g.results) || g.results[g.cur].header != "" {
			return nil
		}
		r := g.results[g.cur]
		if r.captionURL != nil {
			// captions are typed right here, with a live preview
			g.captioning, g.capURL = &r, ""
			g.savedQuery = m.line.Value()
			m.line.Prompt = "caption> "
			m.line.SetValue("")
			return tea.Batch(m.loadGifPreview(), m.placeGifPreview())
		}
		return tea.Batch(m.closeGif(), m.sendGif(r))
	}
	before := m.line.Value()
	var cmd tea.Cmd
	m.line, cmd = m.line.Update(k)
	if m.line.Value() != before {
		// wait for a pause in typing before hitting the API
		g.seq++
		seq := g.seq
		return tea.Batch(cmd, tea.Tick(350*time.Millisecond, func(time.Time) tea.Msg { return gifSearchMsg{seq} }))
	}
	return cmd
}

// closeGif goes back to writing and frees the preview images in the terminal.
func (m *Model) closeGif() tea.Cmd {
	var b strings.Builder
	if m.gif != nil {
		for _, t := range m.gif.preview {
			if t.sent {
				b.WriteString(kittyDelete(t.id))
			}
		}
	}
	m.gif = nil
	m.mode = modeInsert
	m.line.Blur()
	if b.Len() == 0 {
		return m.compose.Focus()
	}
	return tea.Batch(tea.Raw(b.String()), m.compose.Focus())
}

// sendGif downloads the gif and sends it to the open chat as an attachment.
func (m *Model) sendGif(r gifResult) tea.Cmd {
	if m.cur == nil {
		return nil
	}
	chatID := m.cur.ID
	replyTo := ""
	if m.replyTo != nil {
		replyTo = m.replyTo.ID
		m.replyTo = nil
	}
	what := firstNonEmpty(r.kind, "gif")
	// downloads and uploads can take a few seconds, so say so straight away
	m.setStatus("sending " + what + "...")
	return m.run("sent "+what, func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		u, err := resolveURL(ctx, r.sendURL)
		if err != nil {
			return nil, err
		}
		path, err := fetchTemp(ctx, u, "beepertui-media-*")
		if err != nil {
			return nil, err
		}
		named := filepath.Join(filepath.Dir(path), safeName(r.slug)+sniffExt(path))
		if os.Rename(path, named) == nil {
			path = named
		}
		defer os.Remove(path)
		up, err := m.c.Upload(ctx, path)
		if err != nil {
			return nil, err
		}
		up.Kind = r.attType
		if _, err := m.c.SendAttachment(ctx, chatID, up, "", replyTo); err != nil {
			return nil, err
		}
		done := func(m *Model) tea.Cmd { return tea.Batch(m.showToast("✓", "sent "+what), reloadAfter(m)) }
		if r.source != "klipy" {
			return done, nil
		}
		// tell KLIPY it was shared, as their API asks; best effort
		share := klipyBase + url.PathEscape(klipyKey()) + "/gifs/share/" + url.PathEscape(r.slug) + "?customer_id=" + customerID()
		if req, err := http.NewRequestWithContext(ctx, http.MethodPost, share, nil); err == nil {
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}
		return done, nil
	})
}

// sniffExt picks a file extension from the downloaded bytes.
func sniffExt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	switch http.DetectContentType(head[:n]) {
	case "image/gif":
		return ".gif"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	}
	return ""
}

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, s)
	if s == "" {
		return "gif"
	}
	return s
}

func (m *Model) gifGeometry() (h, listW, prevW int) {
	h = m.bodyHeight()
	listW = min(40, m.w/3)
	prevW = max(10, m.w-4-listW-6)
	return
}

// placeGifPreview sizes the highlighted preview's kitty placement to the box.
func (m *Model) placeGifPreview() tea.Cmd {
	g := m.gif
	if g == nil {
		return nil
	}
	key, _ := g.current()
	t := g.preview[key]
	if t == nil || !t.sent {
		return nil
	}
	h, _, prevW := m.gifGeometry()
	c, r := fitCells(t.w, t.h, min(prevW, len(diacritics)), min(h-6, len(diacritics)), m.cellAspect)
	if c == t.placedCols && r == t.placedRows {
		return nil
	}
	t.placedCols, t.placedRows = c, r
	return tea.Raw(kittyPlace(t.id, c, r))
}

func (m *Model) renderGif() string {
	g := m.gif
	h, listW, prevW := m.gifGeometry()

	var list []string
	switch {
	case g.err != nil:
		list = append(list, stErr.Render(truncate(g.err.Error(), listW)))
	case g.loading && len(g.results) == 0:
		list = append(list, stDim.Render("searching..."))
	case len(g.results) == 0:
		list = append(list, stDim.Render("nothing found"))
	}
	start := max(0, g.cur-(h-6)+1)
	for i := start; i < len(g.results) && len(list) < h-5; i++ {
		if hd := g.results[i].header; hd != "" {
			list = append(list, stAccent.Bold(true).Render(truncate(hd, listW)))
			continue
		}
		t := truncate(oneLine(g.results[i].title), listW-2)
		if i == g.cur {
			list = append(list, stCursor.Render("> ")+stBold.Render(t))
		} else {
			list = append(list, "  "+stText.Render(t))
		}
	}

	if g.captioning != nil && g.captioning.captionHelp != nil {
		list = g.captioning.captionHelp(listW)
	}
	var prev []string
	if key, _ := g.current(); key != "" {
		if t := g.preview[key]; t != nil && !t.loading && !t.failed {
			if t.sent && t.placedCols > 0 {
				prev = kittyLines(t.id, t.placedCols, t.placedRows)
			} else if t.img != nil {
				prev = halfBlocks(t.img, prevW, h-6)
			}
		} else {
			prev = []string{stDim.Render("loading preview...")}
		}
	}

	tab := mediaTabs[g.tab]
	credit := tab.credit
	if tab.id == "gifs" {
		credit = map[string]string{"klipy": "KLIPY", "giphy": "GIPHY"}[g.gifProv]
		if credit == "" {
			credit = "needs a free KLIPY or GIPHY key"
		}
	}
	var tabs []string
	for i, t := range mediaTabs {
		if i == g.tab {
			tabs = append(tabs, stTabOn.Render(t.label))
		} else {
			tabs = append(tabs, stTab.Render(t.label))
		}
	}
	title := truncate(strings.Join(tabs, ""), m.w-8) + "\n" +
		stDim.Render("type to search · tab switches · ↑↓ pick · enter sends · esc closes   ") + stDim.Italic(true).Render("via "+credit)
	left := lipgloss.NewStyle().Width(listW).Height(h - 5).Render(strings.Join(list, "\n"))
	right := lipgloss.Place(prevW, h-5, lipgloss.Center, lipgloss.Center, strings.Join(prev, "\n"))
	box := stOverlay.Width(m.w - 4).Render(title + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right))
	return lipgloss.Place(m.w, m.bodyHeight()+2, lipgloss.Center, lipgloss.Center, box)
}
