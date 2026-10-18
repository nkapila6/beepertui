package ui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	Example  struct {
		Text []string `json:"text"`
	} `json:"example"`
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

//go:embed memekeywords.json
var memeKeywordsJSON []byte

// memeKeywords is our own search words per template (memegen's are mostly
// empty): what the meme means, the mood, when people use it, who's in it.
var memeKeywords = func() map[string]string {
	k := map[string]string{}
	json.Unmarshal(memeKeywordsJSON, &k)
	return k
}()

// searchFields is everything a template can be found by, with a weight each.
func (t memeTemplate) searchFields() []struct {
	words  []string
	weight int
} {
	split := func(s string) []string {
		return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
		})
	}
	return []struct {
		words  []string
		weight int
	}{
		{split(t.Name + " " + t.ID), 4},
		{split(memeKeywords[t.ID] + " " + strings.Join(t.Keywords, " ")), 3},
		{split(strings.Join(t.Example.Text, " ")), 1},
	}
}

// memeScore is how well a template matches the query: every typed word has
// to hit something (name, keywords or example caption), and better fields
// and whole-word hits score higher. 0 means no match.
func memeScore(t memeTemplate, q string) int {
	terms := strings.Fields(strings.ToLower(q))
	if len(terms) == 0 {
		return 1
	}
	fields := t.searchFields()
	total := 0
	for _, term := range terms {
		best := 0
		for _, f := range fields {
			for _, w := range f.words {
				switch {
				case w == term:
					best = max(best, f.weight*3)
				case strings.HasPrefix(w, term):
					best = max(best, f.weight*2)
				case len(term) >= 4 && strings.Contains(w, term):
					best = max(best, f.weight)
				}
			}
		}
		if best == 0 {
			return 0
		}
		total += best
	}
	return total
}

// memeSearch ranks templates by memeScore. With no query, animated ones come first.
func (m *Model) memeSearch(ctx context.Context, q string) ([]gifResult, error) {
	ts := m.memeCache
	if ts == nil {
		var err error
		if ts, err = memegenTemplates(ctx); err != nil {
			return nil, err
		}
	}
	type scored struct {
		r     gifResult
		score int
	}
	var hits []scored
	seen := map[string]bool{}
	for _, t := range ts {
		if seen[t.ID] {
			continue // memegen lists a couple of templates twice
		}
		seen[t.ID] = true
		sc := memeScore(t, q)
		if sc == 0 {
			continue
		}
		t := t
		r := gifResult{slug: t.ID, title: t.Name, previewURL: t.Blank, meme: &t}
		if t.animated() {
			r.title += "  (gif)"
			sc++ // nudge animated ones up among equals
		}
		hits = append(hits, scored{r, sc})
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	out := make([]gifResult, len(hits))
	for i, h := range hits {
		out[i] = h.r
	}
	return out, nil
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

type memeCaptionMsg struct{ seq int }

// splitCaption turns "top / bottom" into lines.
func splitCaption(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	if parts := strings.Split(s, " / "); len(parts) > 1 {
		return parts
	}
	return strings.Split(s, "/")
}

// captionKey drives the caption step inside the picker: typing updates a
// live preview (after a short pause), enter sends, esc goes back.
func (m *Model) captionKey(k tea.KeyPressMsg) tea.Cmd {
	g := m.gif
	switch k.String() {
	case "esc":
		g.captioning, g.capURL = nil, ""
		m.line.Prompt = "gif> "
		m.line.SetValue(g.savedQuery)
		m.line.CursorEnd()
		return tea.Batch(m.loadGifPreview(), m.placeGifPreview())
	case "enter":
		r := *g.captioning
		r.sendURL = memeURL(*r.meme, splitCaption(m.line.Value()))
		return tea.Batch(m.closeGif(), m.sendGif(r))
	}
	before := m.line.Value()
	var cmd tea.Cmd
	m.line, cmd = m.line.Update(k)
	if m.line.Value() == before {
		return cmd
	}
	g.seq++
	seq := g.seq
	return tea.Batch(cmd, tea.Tick(450*time.Millisecond, func(time.Time) tea.Msg { return memeCaptionMsg{seq} }))
}

// refreshMemePreview points the preview at the captioned image (always a
// still PNG, so animated templates preview quickly too).
func (m *Model) refreshMemePreview() tea.Cmd {
	g := m.gif
	if g == nil || g.captioning == nil {
		return nil
	}
	lines := splitCaption(m.line.Value())
	if len(lines) == 0 {
		g.capURL = ""
	} else {
		t := *g.captioning.meme
		t.Styles = nil // png preview even for animated templates
		g.capURL = memeURL(t, lines)
	}
	return tea.Batch(m.loadGifPreview(), m.placeGifPreview())
}

// captionHelp explains the slots for this template, with memegen's own example.
func (m *Model) captionHelp(t memeTemplate, w int) []string {
	slots := max(1, t.Lines)
	names := []string{"top", "bottom"}
	if slots != 2 {
		names = nil
		for i := 1; i <= slots; i++ {
			names = append(names, fmt.Sprintf("line %d", i))
		}
	}
	out := []string{
		stBold.Render(truncate(t.Name, w)),
		"",
		stText.Render(fmt.Sprintf("%d text slots, split by /", slots)),
		stAccent.Render("  " + truncate(strings.Join(names, " / "), w-2)),
	}
	if len(t.Example.Text) > 0 {
		out = append(out, "", stDim.Render("for example:"))
		for _, l := range strings.Split(lipgloss.Wrap(strings.Join(t.Example.Text, " / "), w-2, " "), "\n") {
			out = append(out, stAccent.Render("  "+l))
		}
	}
	out = append(out, "",
		stDim.Render("one part fills just the first slot"),
		stDim.Render("the preview updates as you type"),
		"",
		stDim.Render("enter sends, esc back to templates"))
	return out
}
