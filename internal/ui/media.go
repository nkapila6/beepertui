package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The media picker's tabs. Everything past GIFs is free and needs no key.
// Each tab has a command; "all" searches every source at once.

type mediaTab struct {
	id, label, credit, cmd string
}

var mediaTabs = []mediaTab{
	{"all", "All", "everything at once", "media"},
	{"gifs", "GIFs", "", "gif"},
	{"memes", "Memes", "memegen.link", "meme"},
	{"reactions", "Reactions", "nekos.best", "nekos"},
	{"stickers", "Stickers", "Emoji Kitchen, via emojik.vercel.app", "emoji"},
	{"cats", "Cats", "cataas.com", "cataas"},
	{"xkcd", "xkcd", "xkcd.com (CC BY-NC 2.5)", "xkcd"},
	{"status", "Status", "http.cat and http.dog", "status"},
	{"reddit", "Reddit", "meme-api.com", "memeapi"},
}

func tabIndex(id string) int {
	for i, t := range mediaTabs {
		if t.id == id {
			return i
		}
	}
	return 0
}

// how many results each source gets in the All tab
const allPerSource = 5

const userAgent = "beepertui (+https://github.com/nkapila6/beepertui)"

var mediaHTTP = &http.Client{Timeout: 15 * time.Second}

func getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := mediaHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", req.URL.Host, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func getText(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := mediaHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", req.URL.Host, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}

// lists that rarely change (meme templates, categories, tags, the xkcd
// archive) are fetched once per session. Searches run off the UI goroutine,
// hence the lock.
var (
	cacheMu     sync.Mutex
	memeList    []memeTemplate
	nekosCats   []string
	catTags     []string
	xkcdList    []xkcdEntry
	xkcdImages  = map[int]string{}
	searchCache = map[string][]gifResult{}
)

// cached returns a session-cached list, fetching it the first time.
func cached[T any](get func() []T, set func([]T), fetch func() ([]T, error)) ([]T, error) {
	cacheMu.Lock()
	v := get()
	cacheMu.Unlock()
	if len(v) > 0 {
		return v, nil
	}
	v, err := fetch()
	if err != nil {
		return nil, err
	}
	cacheMu.Lock()
	set(v)
	cacheMu.Unlock()
	return v, nil
}

// terms splits a query into lowercase words.
func terms(q string) []string {
	return strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	})
}

// reactions: nekos.best, anime reaction GIFs by category

var nekosSynonyms = map[string][]string{
	"lol": {"laugh"}, "haha": {"laugh"}, "funny": {"laugh"}, "sad": {"cry"}, "tears": {"cry"},
	"hi": {"wave"}, "hello": {"wave"}, "bye": {"wave"}, "yes": {"nod", "thumbsup"}, "no": {"nope"},
	"ok": {"thumbsup"}, "good": {"thumbsup"}, "nice": {"thumbsup"}, "love": {"hug", "kiss", "cuddle"},
	"mad": {"angry", "pout"}, "tired": {"sleep", "yawn"}, "hmm": {"think"}, "idk": {"shrug"},
	"dunno": {"shrug"}, "ugh": {"facepalm"}, "omg": {"stare"}, "dance": {"dance"}, "party": {"dance"},
	"thanks": {"thumbsup", "nod"}, "congrats": {"highfive", "happy"}, "excited": {"happy", "dance"},
}

func nekosCategories(ctx context.Context) ([]string, error) {
	return cached(func() []string { return nekosCats }, func(v []string) { nekosCats = v }, func() ([]string, error) {
		var eps map[string]struct {
			Format string `json:"format"`
		}
		if err := getJSON(ctx, "https://nekos.best/api/v2/endpoints", &eps); err != nil {
			return nil, err
		}
		var out []string
		for k, v := range eps {
			if v.Format == "gif" {
				out = append(out, k)
			}
		}
		sort.Strings(out)
		return out, nil
	})
}

func nekosSearch(ctx context.Context, q string, limit int) ([]gifResult, error) {
	cats, err := nekosCategories(ctx)
	if err != nil {
		return nil, err
	}
	var pick []string
	if ts := terms(q); len(ts) == 0 {
		pick = []string{"laugh", "smile", "wave", "thumbsup", "cry", "facepalm", "shrug", "happy"}
	} else {
		seen := map[string]bool{}
		for _, t := range ts {
			cands := append([]string{}, nekosSynonyms[t]...)
			for _, c := range cats {
				if strings.HasPrefix(c, t) || len(t) >= 3 && strings.Contains(c, t) {
					cands = append(cands, c)
				}
			}
			for _, c := range cands {
				if !seen[c] {
					seen[c] = true
					pick = append(pick, c)
				}
			}
		}
		if len(pick) == 0 {
			return nil, nil
		}
	}
	pick = pick[:min(len(pick), 8)]
	per := max(2, limit/len(pick))
	type res struct {
		Results []struct {
			URL   string `json:"url"`
			Anime string `json:"anime_name"`
		} `json:"results"`
	}
	all := make([][]gifResult, len(pick))
	var wg sync.WaitGroup
	for i, c := range pick {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var r res
			if getJSON(ctx, fmt.Sprintf("https://nekos.best/api/v2/%s?amount=%d", c, per), &r) != nil {
				return
			}
			for _, x := range r.Results {
				all[i] = append(all[i], gifResult{source: "nekos", kind: "gif", slug: c, title: c + " · " + x.Anime, sendURL: x.URL, previewURL: x.URL})
			}
		}()
	}
	wg.Wait()
	var out []gifResult
	for _, a := range all {
		out = append(out, a...)
	}
	return out[:min(len(out), limit)], nil
}

// stickers: Emoji Kitchen mashups

var kitchenPartners = []string{"❤️", "\U0001F602", "\U0001F525", "\U0001F62D", "\U0001F480", "\U0001F97A", "✨", "\U0001F440", "\U0001F60D", "\U0001F389", "\U0001F4AF", "\U0001F914", "\U0001F60E", "\U0001F431", "\U0001F355"}

var kitchenFeatured = [][2]string{
	{"\U0001F62D", "\U0001F525"}, {"\U0001F480", "\U0001F602"}, {"\U0001F97A", "❤️"}, {"\U0001F440", "✨"},
	{"\U0001F60E", "\U0001F525"}, {"\U0001F914", "\U0001F4AF"}, {"\U0001F431", "\U0001F60D"}, {"\U0001F389", "\U0001F602"},
	{"\U0001F355", "\U0001F60D"}, {"\U0001F480", "\U0001F62D"},
}

// kitchenEmoji pulls emoji out of a query: literal emoji, or names like "cry fire" / "skull + laugh".
func kitchenEmoji(q string) []string {
	var out []string
	for _, tok := range strings.FieldsFunc(q, func(r rune) bool { return r == ' ' || r == '+' || r == ',' }) {
		if strings.ContainsFunc(tok, func(r rune) bool { return r > 0x7f }) {
			for _, r := range tok {
				if r > 0x2000 && r != 0xFE0F && r != 0x200D {
					out = append(out, string(r))
				}
			}
			continue
		}
		for _, e := range emojiTable {
			if matchEmoji(pickItem{title: e[0], detail: e[1]}, tok) {
				out = append(out, strings.TrimSuffix(e[0], "️"))
				break
			}
		}
	}
	return out
}

func kitchenURL(a, b string) string {
	return "https://emojik.vercel.app/s/" + url.PathEscape(a) + "_" + url.PathEscape(b) + "?size=256"
}

func kitchenSearch(q string, limit int) []gifResult {
	es := kitchenEmoji(q)
	var pairs [][2]string
	switch {
	case strings.TrimSpace(q) == "":
		pairs = kitchenFeatured
	case len(es) == 0:
		return nil
	case len(es) == 1:
		for _, p := range kitchenPartners {
			if p != es[0] {
				pairs = append(pairs, [2]string{es[0], p})
			}
		}
	default:
		pairs = append(pairs, [2]string{es[0], es[1]})
		for _, p := range kitchenPartners {
			pairs = append(pairs, [2]string{es[0], p})
		}
	}
	var out []gifResult
	for _, p := range pairs[:min(len(pairs), limit)] {
		u := kitchenURL(p[0], p[1])
		name := strings.Fields(emojiName(p[0]) + " ?")[0] + " + " + strings.Fields(emojiName(p[1]) + " ?")[0]
		out = append(out, gifResult{source: "kitchen", kind: "sticker", attType: "sticker", slug: "kitchen-" + p[0] + p[1], title: p[0] + " + " + p[1] + "  " + name, sendURL: u, previewURL: u})
	}
	return out
}

// cats: cataas.com, with optional text on the cat

func cataasTags(ctx context.Context) ([]string, error) {
	return cached(func() []string { return catTags }, func(v []string) { catTags = v }, func() ([]string, error) {
		var tags []string
		if err := getJSON(ctx, "https://cataas.com/api/tags", &tags); err != nil {
			return nil, err
		}
		var out []string
		for _, t := range tags {
			if t = strings.TrimSpace(t); len(t) > 1 && !strings.ContainsAny(t, "#@.") {
				out = append(out, t)
			}
		}
		return out, nil
	})
}

func cataasSearch(ctx context.Context, q string, limit int) ([]gifResult, error) {
	v := url.Values{"limit": {strconv.Itoa(limit)}}
	if ts := terms(q); len(ts) > 0 {
		tags, err := cataasTags(ctx)
		if err != nil {
			return nil, err
		}
		var match []string
		for _, t := range tags {
			lt := strings.ToLower(t)
			for _, w := range ts {
				if lt == w || strings.HasPrefix(lt, w) {
					match = append(match, t)
					break
				}
			}
		}
		if len(match) == 0 {
			return nil, nil
		}
		v.Set("tags", strings.Join(match[:min(len(match), 3)], ","))
	} else {
		v.Set("skip", strconv.Itoa(rand.IntN(800)))
	}
	var cats []struct {
		ID       string   `json:"id"`
		Tags     []string `json:"tags"`
		Mimetype string   `json:"mimetype"`
	}
	if err := getJSON(ctx, "https://cataas.com/api/cats?"+v.Encode(), &cats); err != nil {
		return nil, err
	}
	var out []gifResult
	for _, c := range cats {
		id := c.ID
		base := "https://cataas.com/cat/" + url.PathEscape(id)
		title := "cat"
		if len(c.Tags) > 0 {
			title += " · " + strings.Join(c.Tags[:min(len(c.Tags), 3)], ", ")
		}
		if c.Mimetype == "image/gif" {
			title += "  (gif)"
		}
		says := func(text string) string { return base + "/says/" + url.PathEscape(text) }
		out = append(out, gifResult{
			source: "cataas", kind: "cat", slug: "cat-" + id, title: title, sendURL: base, previewURL: base + "?width=400",
			captionURL: func(text string) string {
				if strings.TrimSpace(text) == "" {
					return base
				}
				return says(text)
			},
			captionPreview: func(text string) string { return says(text) + "?width=400" },
			captionHelp: func(int) []string {
				return []string{stBold.Render("text on the cat"), "", stText.Render("type anything, it goes on the image"), stDim.Render("leave it empty for a plain cat"), "", stDim.Render("enter sends, esc back")}
			},
		})
	}
	return out, nil
}

// xkcd: titles from the archive page, images resolved on demand

type xkcdEntry struct {
	num   int
	title string
}

var xkcdArchiveRe = regexp.MustCompile(`<a href="/(\d+)/" title="[^"]*">([^<]+)</a>`)

func xkcdArchive(ctx context.Context) ([]xkcdEntry, error) {
	return cached(func() []xkcdEntry { return xkcdList }, func(v []xkcdEntry) { xkcdList = v }, func() ([]xkcdEntry, error) {
		page, err := getText(ctx, "https://xkcd.com/archive/")
		if err != nil {
			return nil, err
		}
		var out []xkcdEntry
		for _, m := range xkcdArchiveRe.FindAllStringSubmatch(page, -1) {
			n, _ := strconv.Atoi(m[1])
			out = append(out, xkcdEntry{n, htmlUnescape(m[2])})
		}
		return out, nil
	})
}

func htmlUnescape(s string) string {
	return strings.NewReplacer("&amp;", "&", "&quot;", `"`, "&#39;", "'", "&lt;", "<", "&gt;", ">").Replace(s)
}

func xkcdSearch(ctx context.Context, q string, limit int) ([]gifResult, error) {
	list, err := xkcdArchive(ctx)
	if err != nil {
		return nil, err
	}
	var hits []xkcdEntry
	ts := terms(q)
	switch {
	case len(ts) == 0:
		hits = list[:min(len(list), limit)]
	case len(ts) == 1 && isDigits(ts[0]):
		n, _ := strconv.Atoi(ts[0])
		for _, e := range list {
			if e.num == n || strings.HasPrefix(strconv.Itoa(e.num), ts[0]) {
				hits = append(hits, e)
			}
		}
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].num == n && hits[b].num != n })
	default:
	entries:
		for _, e := range list {
			words := terms(e.title)
			for _, t := range ts {
				ok := false
				for _, w := range words {
					if strings.HasPrefix(w, t) {
						ok = true
						break
					}
				}
				if !ok {
					continue entries
				}
			}
			hits = append(hits, e)
		}
	}
	var out []gifResult
	for _, e := range hits[:min(len(hits), limit)] {
		ref := fmt.Sprintf("xkcd:%d", e.num)
		out = append(out, gifResult{source: "xkcd", kind: "comic", slug: fmt.Sprintf("xkcd-%d", e.num), title: fmt.Sprintf("#%d %s", e.num, e.title), sendURL: ref, previewURL: ref})
	}
	return out, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// resolveURL turns placeholder refs (xkcd:N) into real image URLs.
func resolveURL(ctx context.Context, u string) (string, error) {
	n, ok := strings.CutPrefix(u, "xkcd:")
	if !ok {
		return u, nil
	}
	num, _ := strconv.Atoi(n)
	cacheMu.Lock()
	img := xkcdImages[num]
	cacheMu.Unlock()
	if img != "" {
		return img, nil
	}
	var info struct {
		Img string `json:"img"`
	}
	if err := getJSON(ctx, fmt.Sprintf("https://xkcd.com/%d/info.0.json", num), &info); err != nil {
		return "", err
	}
	if info.Img == "" {
		return "", errors.New("xkcd: no image for that comic")
	}
	cacheMu.Lock()
	xkcdImages[num] = info.Img
	cacheMu.Unlock()
	return info.Img, nil
}

// status: HTTP codes as cats and dogs

var extraStatus = map[int]string{
	420: "Enhance Your Calm", 444: "No Response", 450: "Blocked by Parental Controls", 494: "Request Header Too Large",
	497: "HTTP Request Sent to HTTPS Port", 498: "Token Expired", 499: "Client Closed Request", 509: "Bandwidth Limit Exceeded",
	521: "Web Server Is Down", 522: "Connection Timed Out", 523: "Origin Is Unreachable", 525: "SSL Handshake Failed",
	530: "Site Frozen", 599: "Network Connect Timeout Error",
}

func statusText(code int) string {
	if t := http.StatusText(code); t != "" {
		return t
	}
	return extraStatus[code]
}

func statusSearch(q string, limit int) []gifResult {
	var codes []int
	ts := terms(q)
	if len(ts) == 0 {
		codes = []int{200, 404, 418, 500, 503, 429, 403, 401, 301, 502, 204, 409}
	} else {
		for c := 100; c < 600; c++ {
			text := statusText(c)
			if text == "" {
				continue
			}
			words := terms(text)
			ok := true
			for _, t := range ts {
				hit := strings.HasPrefix(strconv.Itoa(c), t)
				for _, w := range words {
					hit = hit || strings.HasPrefix(w, t)
				}
				ok = ok && hit
			}
			if ok {
				codes = append(codes, c)
			}
		}
	}
	var out []gifResult
	for _, c := range codes {
		for _, a := range []struct{ name, host string }{{"cat", "http.cat"}, {"dog", "http.dog"}} {
			u := fmt.Sprintf("https://%s/%d.jpg", a.host, c)
			out = append(out, gifResult{source: "status", kind: "status " + a.name, slug: fmt.Sprintf("%d-%s", c, a.name), title: fmt.Sprintf("%d %s · %s", c, statusText(c), a.name), sendURL: u, previewURL: u})
		}
		if len(out) >= limit {
			break
		}
	}
	return out[:min(len(out), limit)]
}

// reddit: meme-api.com, a subreddit's top posts

var defaultSubs = []string{"memes", "ProgrammerHumor", "wholesomememes", "me_irl"}

func redditSearch(ctx context.Context, q string, limit int) ([]gifResult, error) {
	subs := defaultSubs
	if ts := strings.Fields(strings.TrimPrefix(strings.TrimSpace(q), "r/")); len(ts) > 0 {
		subs = []string{strings.TrimPrefix(ts[0], "r/")}
	}
	per := max(3, min(50, limit/len(subs)+1))
	type post struct {
		Title, URL, Subreddit string
		NSFW                  bool     `json:"nsfw"`
		Spoiler               bool     `json:"spoiler"`
		Preview               []string `json:"preview"`
	}
	all := make([][]gifResult, len(subs))
	errs := make([]error, len(subs))
	var wg sync.WaitGroup
	for i, s := range subs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var r struct {
				Memes []post `json:"memes"`
			}
			if errs[i] = getJSON(ctx, fmt.Sprintf("https://meme-api.com/gimme/%s/%d", url.PathEscape(s), per), &r); errs[i] != nil {
				return
			}
			for _, p := range r.Memes {
				ext := strings.ToLower(p.URL[strings.LastIndex(p.URL, ".")+1:])
				if p.NSFW || p.Spoiler || (ext != "jpg" && ext != "jpeg" && ext != "png" && ext != "gif") {
					continue
				}
				prev := p.URL
				if len(p.Preview) > 1 {
					prev = p.Preview[len(p.Preview)/2]
				}
				all[i] = append(all[i], gifResult{source: "reddit", kind: "meme", slug: "reddit-" + p.URL[strings.LastIndex(p.URL, "/")+1:], title: "r/" + p.Subreddit + " · " + p.Title, sendURL: p.URL, previewURL: prev})
			}
		}()
	}
	wg.Wait()
	var out []gifResult
	for _, a := range all {
		out = append(out, a...)
	}
	if len(out) == 0 && len(subs) == 1 && errs[0] != nil {
		return nil, fmt.Errorf("couldn't load r/%s", subs[0])
	}
	return out[:min(len(out), limit)], nil
}

// sourceSearch runs one tab's search.
func (m *Model) sourceSearch(ctx context.Context, src, gifProv, q string, limit int) ([]gifResult, error) {
	key := src + "\x00" + q
	cacheMu.Lock()
	hit, ok := searchCache[key]
	cacheMu.Unlock()
	// random-ish sources stay fresh, everything else is cached for the session
	if ok && src != "cats" && src != "reddit" && src != "reactions" {
		return hit[:min(len(hit), limit)], nil
	}
	var res []gifResult
	var err error
	switch src {
	case "gifs":
		switch gifProv {
		case "klipy":
			res, err = klipySearch(ctx, q)
		case "giphy":
			res, err = giphySearch(ctx, q)
		default:
			err = errors.New("GIF search needs a free key: BEEPERTUI_KLIPY_KEY or BEEPERTUI_GIPHY_KEY. The other tabs work without one")
		}
	case "memes":
		res, err = memeSearch(ctx, q)
	case "reactions":
		res, err = nekosSearch(ctx, q, limit)
	case "stickers":
		res = kitchenSearch(q, limit)
	case "cats":
		res, err = cataasSearch(ctx, q, limit)
	case "xkcd":
		res, err = xkcdSearch(ctx, q, limit)
	case "status":
		res = statusSearch(q, limit)
	case "reddit":
		res, err = redditSearch(ctx, q, limit)
	}
	if err == nil {
		cacheMu.Lock()
		searchCache[key] = res
		cacheMu.Unlock()
	}
	return res[:min(len(res), limit)], err
}

// allSearch asks every source at once and returns their top hits in
// sections. A source that fails or finds nothing just doesn't show up.
func (m *Model) allSearch(ctx context.Context, gifProv, q string) ([]gifResult, error) {
	tabs := mediaTabs[1:]
	parts := make([][]gifResult, len(tabs))
	var wg sync.WaitGroup
	for i, t := range tabs {
		if t.id == "gifs" && gifProv == "" {
			continue
		}
		if t.id == "reddit" && strings.TrimSpace(q) != "" && len(strings.Fields(q)) > 1 {
			continue // a multi-word query isn't a subreddit
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			parts[i], _ = m.sourceSearch(c, t.id, gifProv, q, allPerSource)
		}()
	}
	wg.Wait()
	var out []gifResult
	for i, t := range tabs {
		if len(parts[i]) == 0 {
			continue
		}
		out = append(out, gifResult{header: fmt.Sprintf("%s  :%s", t.label, t.cmd)})
		out = append(out, parts[i]...)
	}
	return out, nil
}
