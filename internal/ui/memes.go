package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Memes from memegen.link: free, open source, no key. They power the GIF
// picker when no KLIPY/GIPHY key is set, and :meme opens them any time.
// Animated templates go out as GIFs.

var memegenBase = firstNonEmpty(os.Getenv("BEEPERTUI_MEMEGEN_URL"), "https://api.memegen.link/")

type memeTemplate struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Lines    int      `json:"lines"`
	Styles   []string `json:"styles"`
	Blank    string   `json:"blank"`
	Keywords []string `json:"keywords"`
}

func (t memeTemplate) animated() bool {
	for _, s := range t.Styles {
		if s == "animated" {
			return true
		}
	}
	return false
}

func memegenTemplates(ctx context.Context) ([]memeTemplate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, memegenBase+"templates/", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("memegen: %s", resp.Status)
	}
	var ts []memeTemplate
	return ts, json.NewDecoder(resp.Body).Decode(&ts)
}

// memeSearch filters the template list locally: animated ones first, then
// matches on id, name or keywords.
func (m *Model) memeSearch(ctx context.Context, q string) ([]gifResult, error) {
	ts := m.memeCache
	if ts == nil {
		var err error
		if ts, err = memegenTemplates(ctx); err != nil {
			return nil, err
		}
	}
	q = strings.ToLower(strings.TrimSpace(q))
	var anim, still []gifResult
	for _, t := range ts {
		hay := strings.ToLower(t.ID + " " + t.Name + " " + strings.Join(t.Keywords, " "))
		if q != "" && !strings.Contains(hay, q) {
			continue
		}
		r := gifResult{slug: t.ID, title: t.Name, previewURL: t.Blank, meme: &t}
		if t.animated() {
			r.title += "  (gif)"
			anim = append(anim, r)
		} else {
			still = append(still, r)
		}
	}
	return append(anim, still...), nil
}

// memeURL builds the image URL for a template and captions, using memegen's
// escaping (spaces are _, a real _ is __, and so on).
func memeURL(t memeTemplate, lines []string) string {
	esc := strings.NewReplacer("_", "__", "-", "--", " ", "_", "?", "~q", "&", "~a", "%", "~p", "#", "~h", "/", "~s", "\\", "~b", "<", "~l", ">", "~g", `"`, "''")
	parts := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			l = "_"
		} else {
			l = url.PathEscape(esc.Replace(l))
		}
		parts = append(parts, l)
	}
	ext := ".png"
	if t.animated() {
		ext = ".gif"
	}
	path := t.ID
	if len(parts) > 0 {
		path += "/" + strings.Join(parts, "/")
	}
	return memegenBase + "images/" + path + ext
}

// captionMeme asks for the captions, then sends the meme.
func (m *Model) captionMeme(r gifResult) tea.Cmd {
	t := *r.meme
	hint := "top / bottom"
	if t.Lines > 2 {
		hint = fmt.Sprintf("%d lines split by /", t.Lines)
	}
	return m.prompt("caption ("+hint+", empty for none)", "", func(m *Model, s string) tea.Cmd {
		var lines []string
		if strings.TrimSpace(s) != "" {
			lines = strings.Split(s, " / ")
			if len(lines) == 1 {
				lines = strings.Split(s, "/")
			}
		}
		r.sendURL = memeURL(t, lines)
		cmd := m.startInsert()
		return tea.Batch(cmd, m.sendGif(r))
	})
}
