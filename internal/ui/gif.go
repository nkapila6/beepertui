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
	sendURL       string // the gif that gets uploaded to the chat
	previewURL    string // a small still for the picker
	width, height int
}

type gifPicker struct {
	query    string
	results  []gifResult
	cur      int
	loading  bool
	err      error
	seq      int // ignores responses to stale queries
	preview  map[string]*thumb
	provider string
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

func providerName(p string) string {
	if p == "giphy" {
		return "GIPHY"
	}
	return "KLIPY"
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

func (m *Model) openGifPicker(query string) tea.Cmd {
	if m.cur == nil {
		m.setErr("open a chat first")
		return nil
	}
	if m.gifProvider() == "" {
		m.setErr("GIF search needs a free key: BEEPERTUI_KLIPY_KEY (partner.klipy.com) or BEEPERTUI_GIPHY_KEY (developers.giphy.com)")
		return nil
	}
	m.gif = &gifPicker{query: query, preview: map[string]*thumb{}, provider: m.gifProvider()}
	m.mode = modeGif
	m.line.Prompt = "gif> "
	m.line.SetValue(query)
	m.line.CursorEnd()
	return tea.Batch(m.line.Focus(), m.searchGifs())
}

func (m *Model) searchGifs() tea.Cmd {
	g := m.gif
	g.seq++
	seq, q, provider := g.seq, strings.TrimSpace(m.line.Value()), g.provider
	g.loading = true
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		search := klipySearch
		if provider == "giphy" {
			search = giphySearch
		}
		res, err := search(ctx, q)
		return func(m *Model) tea.Cmd {
			if m.gif == nil || m.gif.seq != seq {
				return nil
			}
			m.gif.loading, m.gif.err = false, err
			m.gif.results, m.gif.cur = res, 0
			return m.loadGifPreview()
		}, nil
	})
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
		r := gifResult{slug: d.Slug, title: firstNonEmpty(d.Title, d.Slug), sendURL: send.URL, width: send.Width, height: send.Height}
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
		res = append(res, gifResult{slug: d.Slug, title: firstNonEmpty(d.Title, d.Slug), sendURL: send.URL, previewURL: d.Images.FixedHeightStill.URL, width: w, height: h})
	}
	return res, nil
}

// loadGifPreview fetches the highlighted result's still for the preview.
func (m *Model) loadGifPreview() tea.Cmd {
	g := m.gif
	if g == nil || g.cur >= len(g.results) {
		return nil
	}
	r := g.results[g.cur]
	if r.previewURL == "" || g.preview[r.slug] != nil {
		return nil
	}
	t := &thumb{loading: true}
	g.preview[r.slug] = t
	kitty := m.imageMode == imagesKitty
	if kitty {
		m.nextImageID++
		t.id = kittyFirstID + 0x10000 + m.nextImageID
	}
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		path, err := fetchTemp(ctx, r.previewURL, "beepertui-gifprev-*.jpg")
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
	switch k.String() {
	case "esc":
		return m.closeGif()
	case "up", "ctrl+p", "ctrl+k":
		if len(g.results) > 0 {
			g.cur = (g.cur - 1 + len(g.results)) % len(g.results)
		}
		return tea.Batch(m.loadGifPreview(), m.placeGifPreview())
	case "down", "ctrl+n", "ctrl+j", "tab":
		if len(g.results) > 0 {
			g.cur = (g.cur + 1) % len(g.results)
		}
		return tea.Batch(m.loadGifPreview(), m.placeGifPreview())
	case "enter":
		if g.cur >= len(g.results) {
			return nil
		}
		r := g.results[g.cur]
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
	provider := m.gifProvider()
	chatID := m.cur.ID
	replyTo := ""
	if m.replyTo != nil {
		replyTo = m.replyTo.ID
		m.replyTo = nil
	}
	return m.run("sent gif", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		path, err := fetchTemp(ctx, r.sendURL, "beepertui-gif-*.gif")
		if err != nil {
			return nil, err
		}
		named := filepath.Join(filepath.Dir(path), safeName(r.slug)+".gif")
		if os.Rename(path, named) == nil {
			path = named
		}
		defer os.Remove(path)
		up, err := m.c.Upload(ctx, path)
		if err != nil {
			return nil, err
		}
		if _, err := m.c.SendAttachment(ctx, chatID, up, "", replyTo); err != nil {
			return nil, err
		}
		if provider != "klipy" {
			return reloadAfter, nil
		}
		// tell KLIPY it was shared, as their API asks; best effort
		share := klipyBase + url.PathEscape(klipyKey()) + "/gifs/share/" + url.PathEscape(r.slug) + "?customer_id=" + customerID()
		if req, err := http.NewRequestWithContext(ctx, http.MethodPost, share, nil); err == nil {
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}
		return reloadAfter, nil
	})
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
	if g == nil || g.cur >= len(g.results) {
		return nil
	}
	t := g.preview[g.results[g.cur].slug]
	if t == nil || !t.sent {
		return nil
	}
	h, _, prevW := m.gifGeometry()
	c, r := fitCells(t.w, t.h, min(prevW, len(diacritics)), min(h-4, len(diacritics)), m.cellAspect)
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
	start := max(0, g.cur-(h-4)+1)
	for i := start; i < len(g.results) && len(list) < h-3; i++ {
		t := truncate(oneLine(g.results[i].title), listW-2)
		if i == g.cur {
			list = append(list, stCursor.Render("> ")+stBold.Render(t))
		} else {
			list = append(list, "  "+stText.Render(t))
		}
	}

	var prev []string
	if g.cur < len(g.results) {
		if t := g.preview[g.results[g.cur].slug]; t != nil && !t.loading && !t.failed {
			if t.sent && t.placedCols > 0 {
				prev = kittyLines(t.id, t.placedCols, t.placedRows)
			} else if t.img != nil {
				prev = halfBlocks(t.img, prevW, h-4)
			}
		} else {
			prev = []string{stDim.Render("loading preview...")}
		}
	}

	title := stBold.Render("GIFs") + stDim.Render("   type to search, enter sends, esc cancels   ") + stDim.Italic(true).Render("Powered by "+providerName(g.provider))
	left := lipgloss.NewStyle().Width(listW).Height(h - 3).Render(strings.Join(list, "\n"))
	right := lipgloss.Place(prevW, h-3, lipgloss.Center, lipgloss.Center, strings.Join(prev, "\n"))
	box := stOverlay.Width(m.w - 4).Render(title + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right))
	return lipgloss.Place(m.w, m.bodyHeight()+2, lipgloss.Center, lipgloss.Center, box)
}
