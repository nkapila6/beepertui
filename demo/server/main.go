// Command server is a fake Beeper Desktop API with made-up chats, used to
// record the demo videos. Everyone and everything in it is fictional.
//
//	go run ./demo/server -image path/to/picture.png
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type obj = map[string]any

// Beeper turns markdown links (mentions included) into rich text; do the same
var mdLinks = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

var (
	challenge string
	partyGIF  string                // a generated animated gif, so the demo shows playback
	uploads   = map[string]string{} // uploadID -> saved file, so sent images show up

	mu        sync.Mutex
	chats     []obj
	messages  = map[string][]obj{}
	seq       = 1000
	imagePath = flag.String("image", "", "local image served for picture messages and avatars")
	addr      = flag.String("addr", "127.0.0.1:23999", "listen address")
	delay     = flag.Duration("send-delay", 0, "how long sent messages take to show up, like a slow network")
	videoPath = flag.String("video", "", "local video posted in Weekend Hike, with no poster so the app makes one")
)

func ts(ago time.Duration) string { return time.Now().Add(-ago).Format(time.RFC3339) }

func person(id, name string) obj { return obj{"id": id, "fullName": name} }

var me = obj{"id": "me", "fullName": "Nikhil", "isSelf": true}

func addChat(id, title, network, typ string, unread int, people []obj, extra obj) {
	c := obj{
		"id": id, "accountID": strings.ToLower(network), "network": network, "title": title, "type": typ,
		"unreadCount": unread, "participants": obj{"items": append([]obj{me}, people...), "hasMore": false, "total": len(people) + 1},
	}
	for k, v := range extra {
		c[k] = v
	}
	chats = append(chats, c)
}

// msg adds a message; from "" means me.
func msg(chat, from, name string, ago time.Duration, text string, extra obj) {
	seq++
	m := obj{
		"id": fmt.Sprintf("m%d", seq), "chatID": chat, "accountID": "x", "senderID": firstOr(from, "me"),
		"senderName": firstOr(name, "Nikhil"), "timestamp": ts(ago), "sortKey": fmt.Sprintf("%d", seq),
		"type": "TEXT", "text": text, "isSender": from == "",
	}
	if from == "" {
		m["sendStatus"] = obj{"status": "SUCCESS"}
		m["seen"] = ago > 20*time.Minute
	}
	for k, v := range extra {
		m[k] = v
	}
	messages[chat] = append(messages[chat], m)
}

func firstOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func react(key, who string) obj {
	return obj{"id": who + key, "reactionKey": key, "participantID": who}
}

func seed() {
	h, mn := time.Hour, time.Minute
	img := "file://" + *imagePath
	avatar := func(ok bool) obj {
		if ok && *imagePath != "" {
			return obj{"imgURL": *imagePath}
		}
		return obj{}
	}

	priya, sam, lena, omar := person("priya", "Gaurav Gosain"), person("sam", "CSK Thala"), person("lena", "Fazalullah"), person("omar", "Omar Haddad")
	addChat("design", "Design Team", "Slack", "group", 3, []obj{priya, sam, lena}, obj{"isPinned": true, "lastReadMessageSortKey": "1006"})
	msg("design", "priya", "Gaurav Gosain", 26*h, "morning all, new onboarding mocks are in figma", nil)
	msg("design", "sam", "CSK Thala", 25*h, "looking now", nil)
	msg("design", "", "", 3*h, "the empty states look <b>so</b> much better", obj{"seen": obj{"priya": ts(170 * mn), "sam": ts(160 * mn), "lena": ts(150 * mn)}})
	msg("design", "lena", "Fazalullah", 2*h+40*mn, `recording the demos with <a href="https://github.com/charmbracelet/vhs">vhs</a> now`, obj{
		"links": []obj{{"url": "https://github.com/charmbracelet/vhs", "title": "charmbracelet/vhs", "summary": "Your CLI home video recorder. Write terminal GIFs as code."}},
	})
	msg("design", "sam", "CSK Thala", 2*h+30*mn, "looks clean, ship it", obj{"reactions": []obj{react("\U0001F525", "priya"), react("\U0001F525", "lena")}})
	msg("design", "", "", 2*h, "been using it all morning tbh", obj{"seen": obj{"priya": ts(110 * mn), "sam": ts(95 * mn), "lena": false}})
	msg("design", "priya", "Gaurav Gosain", 40*mn, "ok here's the final hero for the landing page", obj{
		"type": "IMAGE", "attachments": []obj{{"id": "a1", "type": "img", "srcURL": img, "mimeType": "image/png", "fileName": "hero-final.png"}},
	})
	msg("design", "lena", "Fazalullah", 30*mn, "love the gradient", obj{"linkedMessageID": "m1007"})
	msg("design", "lena", "Fazalullah", 25*mn, "", obj{
		"type": "IMAGE", "attachments": []obj{{"id": "a2", "type": "img", "srcURL": "mxc://demo/party", "mimeType": "image/gif", "fileName": "party.gif", "isGif": true}},
		"reactions": []obj{react("\U0001F389", "sam"), react("\U0001F389", "priya"), react("\U0001F389", "me")},
	})
	msg("design", "sam", "CSK Thala", 12*mn, `<a href="https://matrix.to/#/@me">Nikhil</a> can you review before standup?`, nil)

	addChat("hike", "Weekend Hike", "WhatsApp", "group", 5, []obj{omar, lena, priya}, avatar(true))
	msg("hike", "omar", "Omar Haddad", 5*h, "trail opens at 7, who's in?", nil)
	msg("hike", "lena", "Fazalullah", 4*h, "me! bringing snacks", obj{"reactions": []obj{react("\U0001F44D", "omar")}})
	msg("hike", "priya", "Gaurav Gosain", 3*h, "count me in, I'll drive", nil)
	msg("hike", "omar", "Omar Haddad", 50*mn, "forecast says clear skies", nil)
	msg("hike", "omar", "Omar Haddad", 48*mn, "meeting at the north parking lot", nil)
	if *videoPath != "" {
		msg("hike", "lena", "Fazalullah", 40*mn, "view from the top last time", obj{
			"type": "VIDEO", "attachments": []obj{{"id": "v1", "type": "video", "srcURL": "file://" + *videoPath, "mimeType": "video/mp4", "fileName": "summit.mp4"}},
		})
	}

	addChat("mom", "Mom", "iMessage", "single", 1, []obj{person("mom", "Mom")}, nil)
	msg("mom", "mom", "Mom", 30*h, "did you eat?", nil)
	msg("mom", "", "", 29*h, "yes mom", obj{"reactions": []obj{react("❤️", "mom")}})
	msg("mom", "mom", "Mom", 2*h, "call me this weekend", nil)

	alex := person("alex", "Alex Chen")
	addChat("alex", "Alex Chen", "Telegram", "single", 0, []obj{alex}, avatar(true))
	msg("alex", "alex", "Alex Chen", 6*h, "are we still on for lunch friday?", nil)
	msg("alex", "", "", 5*h, "yes! that ramen place?", nil)
	msg("alex", "alex", "Alex Chen", 5*h-10*mn, "perfect, 12:30", obj{"reactions": []obj{react("\U0001F44D", "me")}})

	addChat("releases", "#releases", "Discord", "group", 2, []obj{person("bot", "release-bot"), sam}, obj{"isMuted": true})
	msg("releases", "bot", "release-bot", 7*h, "v2.4.0 shipped to production", nil)
	msg("releases", "sam", "CSK Thala", 6*h, "changelog: https://example.com/changelog", nil)

	addChat("priya", "Gaurav Gosain", "Signal", "single", 0, []obj{priya}, avatar(true))
	msg("priya", "priya", "Gaurav Gosain", 9*h, "sending you the slides now", nil)
	msg("priya", "", "", 8*h, "got them, thanks!", nil)

	addChat("book", "Book Club", "Messenger", "group", 0, []obj{lena, omar}, nil)
	msg("book", "lena", "Fazalullah", 26*h, "next pick: Project Hail Mary?", obj{"reactions": []obj{react("\U0001F680", "omar"), react("\U0001F680", "me")}})

	addChat("climb", "Climbing Crew", "Telegram", "group", 0, []obj{omar, sam}, nil)
	msg("climb", "sam", "CSK Thala", 50*h, "bouldering thursday?", nil)

	addChat("recruiting", "Jordan Lee", "LinkedIn", "single", 0, []obj{person("jordan", "Jordan Lee")}, nil)
	msg("recruiting", "jordan", "Jordan Lee", 72*h, "hey! saw your post about terminal tools", nil)

	for _, c := range chats {
		ms := messages[c["id"].(string)]
		if len(ms) > 0 {
			last := ms[len(ms)-1]
			c["preview"] = last
			c["lastActivity"] = last["timestamp"]
		}
	}
}

func main() {
	flag.Parse()
	partyGIF = makePartyGIF()
	seed()
	j := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	// the live socket, so the app shows "live" like it does with real Beeper.
	// It sits outside the big lock, since the connection stays open.
	http.HandleFunc("/v1/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		if c.Write(ctx, websocket.MessageText, []byte(`{"type":"ready"}`)) != nil {
			return
		}
		wsMu.Lock()
		sockets[c] = true
		wsMu.Unlock()
		defer func() {
			wsMu.Lock()
			delete(sockets, c)
			wsMu.Unlock()
		}()
		for {
			if _, _, err := c.Read(ctx); err != nil {
				return
			}
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		p := r.URL.Path
		parts := strings.Split(strings.Trim(p, "/"), "/")
		switch {
		case p == "/v1/chats" || p == "/v1/chats/search":
			q := strings.ToLower(r.URL.Query().Get("query"))
			inbox := r.URL.Query().Get("inbox")
			var out []obj
			for _, c := range chats {
				if q != "" && !strings.Contains(strings.ToLower(c["title"].(string)), q) {
					continue
				}
				if inbox == "archive" || inbox == "low-priority" {
					continue
				}
				if r.URL.Query().Get("unreadOnly") == "true" && c["unreadCount"].(int) == 0 {
					continue
				}
				out = append(out, c)
			}
			sort.SliceStable(out, func(a, b int) bool {
				pa, _ := out[a]["isPinned"].(bool)
				pb, _ := out[b]["isPinned"].(bool)
				if pa != pb {
					return pa
				}
				return out[a]["lastActivity"].(string) > out[b]["lastActivity"].(string)
			})
			j(w, obj{"items": out, "hasMore": false, "oldestCursor": "", "newestCursor": ""})
		case len(parts) == 3 && parts[1] == "chats" && r.Method == "GET":
			for _, c := range chats {
				if c["id"] == parts[2] {
					j(w, c)
					return
				}
			}
			http.NotFound(w, r)
		case len(parts) == 4 && parts[3] == "messages" && r.Method == "GET":
			j(w, obj{"items": messages[parts[2]], "hasMore": false, "oldestCursor": "", "newestCursor": ""})
		case len(parts) == 4 && parts[3] == "messages" && r.Method == "POST":
			var body struct {
				Text       string `json:"text"`
				ReplyTo    string `json:"replyToMessageID"`
				Attachment *struct {
					UploadID string `json:"uploadID"`
					MimeType string `json:"mimeType"`
					FileName string `json:"fileName"`
				} `json:"attachment"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			extra := obj{}
			if body.ReplyTo != "" {
				extra["linkedMessageID"] = body.ReplyTo
			}
			if a := body.Attachment; a != nil && uploads[a.UploadID] != "" {
				extra["type"] = "IMAGE"
				extra["attachments"] = []obj{{"id": a.UploadID, "type": "img", "srcURL": "file://" + uploads[a.UploadID],
					"mimeType": a.MimeType, "fileName": a.FileName, "isGif": a.MimeType == "image/gif"}}
			}
			add := func() {
				msg(parts[2], "", "", 0, mdLinks.ReplaceAllString(body.Text, `<a href="$2">$1</a>`), extra)
				bump(parts[2])
				push("message.upserted", parts[2])
			}
			if *delay > 0 {
				go func() {
					time.Sleep(*delay)
					mu.Lock()
					defer mu.Unlock()
					add()
				}()
			} else {
				add()
			}
			j(w, obj{"chatID": parts[2], "pendingMessageID": "p"})
		case len(parts) == 5 && parts[3] == "messages" && r.Method == "PUT":
			var body struct {
				Text string `json:"text"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			for _, m := range messages[parts[2]] {
				if m["id"] == parts[4] {
					m["text"], m["editedTimestamp"] = body.Text, ts(0)
				}
			}
			j(w, obj{})
		case len(parts) == 6 && parts[5] == "reactions" && r.Method == "POST":
			var body struct {
				Key string `json:"reactionKey"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			for _, m := range messages[parts[2]] {
				if m["id"] == parts[4] {
					rs, _ := m["reactions"].([]obj)
					m["reactions"] = append(rs, react(body.Key, "me"))
				}
			}
			j(w, obj{})
		case len(parts) == 4 && parts[3] == "read":
			for _, c := range chats {
				if c["id"] == parts[2] {
					c["unreadCount"] = 0
				}
			}
			j(w, obj{})
		case len(parts) == 3 && parts[1] == "chats" && r.Method == "PATCH":
			var body obj
			json.NewDecoder(r.Body).Decode(&body)
			for _, c := range chats {
				if c["id"] == parts[2] {
					for k, v := range body {
						c[k] = v
					}
				}
			}
			j(w, obj{})
		case p == "/oauth/register":
			j(w, obj{"client_id": "demo-client"})
		case p == "/oauth/authorize":
			// auto-approve, remembering the PKCE challenge for the token step
			q := r.URL.Query()
			challenge = q.Get("code_challenge")
			http.Redirect(w, r, q.Get("redirect_uri")+"?code=demo-code&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
		case p == "/oauth/token":
			r.ParseForm()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "demo-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			j(w, obj{"access_token": "demo-token", "token_type": "Bearer", "expires_in": 3600})
		case p == "/v1/messages/search":
			q := strings.ToLower(r.URL.Query().Get("query"))
			var hits []obj
			for chatID, ms := range messages {
				if want := r.URL.Query()["chatIDs"]; len(want) > 0 && want[0] != chatID {
					continue
				}
				for _, m := range ms {
					if strings.Contains(strings.ToLower(fmt.Sprint(m["text"])), q) {
						hits = append(hits, m)
					}
				}
			}
			j(w, obj{"items": hits, "hasMore": false})
		case p == "/demo/incoming":
			// pretend someone else sent a message: /demo/incoming?chat=hike&from=omar&name=Omar+Haddad&text=hi
			q := r.URL.Query()
			msg(q.Get("chat"), q.Get("from"), q.Get("name"), 0, q.Get("text"), nil)
			bump(q.Get("chat"))
			push("message.upserted", q.Get("chat"))
			for _, c := range chats {
				if c["id"] == q.Get("chat") {
					c["unreadCount"] = c["unreadCount"].(int) + 1
				}
			}
			j(w, obj{})
		case p == "/v1/info":
			j(w, obj{"app": obj{"name": "Beeper (demo)"}})
		case strings.HasPrefix(p, "/klipy/") && (strings.HasSuffix(p, "/gifs/search") || strings.HasSuffix(p, "/gifs/trending")):
			// stand-in for KLIPY's search, every result is the demo image
			media := "http://" + *addr + "/media/demo"
			var items []obj
			for i, t := range []string{"party parrot", "thumbs up", "deal with it", "this is fine", "mind blown", "high five"} {
				f := obj{"gif": obj{"url": media + ".gif", "width": 400, "height": 400}, "jpg": obj{"url": media + ".jpg", "width": 200, "height": 200}}
				items = append(items, obj{"slug": fmt.Sprintf("demo-%d", i), "title": t, "file": obj{"md": f, "sm": f}})
			}
			j(w, obj{"result": true, "data": obj{"data": items}})
		case strings.HasPrefix(p, "/giphy/"):
			media := "http://" + *addr + "/media/demo"
			var items []obj
			for i, t := range []string{"cat typing", "excited dog", "slow clap"} {
				r := obj{"url": media + ".gif", "width": "400", "height": "400"}
				items = append(items, obj{"slug": fmt.Sprintf("giphy-%d", i), "title": t, "images": obj{"downsized": r, "fixed_height": r, "fixed_height_still": obj{"url": media + ".jpg"}}})
			}
			j(w, obj{"data": items})
		case strings.HasPrefix(p, "/media/"):
			http.ServeFile(w, r, *imagePath)
		case p == "/v1/assets/upload":
			f, hdr, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer f.Close()
			out, err := os.CreateTemp("", "demo-upload-*")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			io.Copy(out, f)
			out.Close()
			id := fmt.Sprintf("up%d", len(uploads)+1)
			uploads[id] = out.Name()
			j(w, obj{"uploadID": id, "fileName": hdr.Filename, "mimeType": hdr.Header.Get("Content-Type")})
		case p == "/v1/accounts":
			j(w, []obj{{"accountID": "x", "network": "WhatsApp", "status": "connected", "user": me}})
		case p == "/v1/assets/download":
			var body struct {
				URL string `json:"url"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if strings.Contains(body.URL, "party") {
				j(w, obj{"srcURL": "file://" + partyGIF})
				return
			}
			j(w, obj{"srcURL": "file://" + *imagePath})
		case p == "/v1/ws":
			http.Error(w, "no websocket in the demo", http.StatusBadRequest)
		default:
			io.Copy(io.Discard, r.Body)
			j(w, obj{})
		}
	})
	log.Printf("demo beeper api on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func bump(chat string) {
	for _, c := range chats {
		if c["id"] == chat {
			ms := messages[chat]
			c["preview"] = ms[len(ms)-1]
			c["lastActivity"] = ms[len(ms)-1]["timestamp"]
		}
	}
}

// makePartyGIF draws a small looping animation (bouncing, colour-shifting dots)
// and returns its path. It keeps the demo free of anyone else's media.
func makePartyGIF() string {
	const w, h, frames = 240, 140, 24
	pal := color.Palette{color.RGBA{24, 23, 36, 255}}
	for i := 0; i < 60; i++ {
		t := float64(i) / 60
		pal = append(pal, color.RGBA{uint8(120 + 135*math.Sin(t*6.28)), uint8(120 + 135*math.Sin(t*6.28+2.1)), uint8(120 + 135*math.Sin(t*6.28+4.2)), 255})
	}
	anim := &gif.GIF{}
	for f := 0; f < frames; f++ {
		img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
		ph := float64(f) / frames * 2 * math.Pi
		for d := 0; d < 5; d++ {
			cx := 30 + d*45
			cy := 70 + int(40*math.Sin(ph+float64(d)*0.9))
			ci := uint8(1 + (f*2+d*12)%60)
			for y := -14; y <= 14; y++ {
				for x := -14; x <= 14; x++ {
					if x*x+y*y <= 196 {
						img.SetColorIndex(cx+x, cy+y, ci)
					}
				}
			}
		}
		anim.Image = append(anim.Image, img)
		anim.Delay = append(anim.Delay, 6)
	}
	f, err := os.CreateTemp("", "demo-party-*.gif")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, anim); err != nil {
		log.Fatal(err)
	}
	return f.Name()
}

var (
	wsMu    sync.Mutex
	sockets = map[*websocket.Conn]bool{}
)

// push tells every connected app that something changed in a chat.
func push(typ, chatID string) {
	ev, _ := json.Marshal(obj{"type": typ, "chatID": chatID})
	wsMu.Lock()
	defer wsMu.Unlock()
	for c := range sockets {
		go func(c *websocket.Conn) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			c.Write(ctx, websocket.MessageText, ev)
		}(c)
	}
}
