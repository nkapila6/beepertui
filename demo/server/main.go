// Command server is a fake Beeper Desktop API with made-up chats, used to
// record the demo videos. Everyone and everything in it is fictional.
//
//	go run ./demo/server -image path/to/picture.png
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type obj = map[string]any

var (
	challenge string

	mu       sync.Mutex
	chats    []obj
	messages = map[string][]obj{}
	seq      = 1000
	image    = flag.String("image", "", "local image served for picture messages and avatars")
	addr     = flag.String("addr", "127.0.0.1:23999", "listen address")
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
	img := "file://" + *image
	avatar := func(ok bool) obj {
		if ok && *image != "" {
			return obj{"imgURL": *image}
		}
		return obj{}
	}

	priya, sam, lena, omar := person("priya", "Priya Shah"), person("sam", "Sam Okafor"), person("lena", "Lena Fischer"), person("omar", "Omar Haddad")
	addChat("design", "Design Team", "Slack", "group", 3, []obj{priya, sam, lena}, obj{"isPinned": true, "lastReadMessageSortKey": "1006"})
	msg("design", "priya", "Priya Shah", 26*h, "morning all, new onboarding mocks are in figma", nil)
	msg("design", "sam", "Sam Okafor", 25*h, "looking now", nil)
	msg("design", "", "", 3*h, "the empty states look <b>so</b> much better", nil)
	msg("design", "lena", "Lena Fischer", 2*h+40*mn, `did anyone see the new <a href="https://github.com/nkapila6/beepertui">terminal client</a> for beeper?`, obj{
		"links": []obj{{"url": "https://github.com/nkapila6/beepertui", "title": "nkapila6/beepertui", "summary": "A terminal client for Beeper Desktop with vim keys and flash.nvim-style labels."}},
	})
	msg("design", "sam", "Sam Okafor", 2*h+30*mn, "flash labels for replying is such a good idea", obj{"reactions": []obj{react("\U0001F525", "priya"), react("\U0001F525", "lena")}})
	msg("design", "", "", 2*h, "been using it all morning tbh", nil)
	msg("design", "priya", "Priya Shah", 40*mn, "ok here's the final hero for the landing page", obj{
		"type": "IMAGE", "attachments": []obj{{"id": "a1", "type": "img", "srcURL": img, "mimeType": "image/png", "fileName": "hero-final.png"}},
	})
	msg("design", "lena", "Lena Fischer", 30*mn, "love the gradient", obj{"linkedMessageID": "m1007"})
	msg("design", "sam", "Sam Okafor", 12*mn, `<a href="https://matrix.to/#/@me">Nikhil</a> can you review before standup?`, nil)

	addChat("hike", "Weekend Hike", "WhatsApp", "group", 5, []obj{omar, lena, priya}, avatar(true))
	msg("hike", "omar", "Omar Haddad", 5*h, "trail opens at 7, who's in?", nil)
	msg("hike", "lena", "Lena Fischer", 4*h, "me! bringing snacks", obj{"reactions": []obj{react("\U0001F44D", "omar")}})
	msg("hike", "priya", "Priya Shah", 3*h, "count me in, I'll drive", nil)
	msg("hike", "omar", "Omar Haddad", 50*mn, "forecast says clear skies", nil)
	msg("hike", "omar", "Omar Haddad", 48*mn, "meeting at the north parking lot", nil)

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
	msg("releases", "sam", "Sam Okafor", 6*h, "changelog: https://example.com/changelog", nil)

	addChat("priya", "Priya Shah", "Signal", "single", 0, []obj{priya}, avatar(true))
	msg("priya", "priya", "Priya Shah", 9*h, "sending you the slides now", nil)
	msg("priya", "", "", 8*h, "got them, thanks!", nil)

	addChat("book", "Book Club", "Messenger", "group", 0, []obj{lena, omar}, nil)
	msg("book", "lena", "Lena Fischer", 26*h, "next pick: Project Hail Mary?", obj{"reactions": []obj{react("\U0001F680", "omar"), react("\U0001F680", "me")}})

	addChat("climb", "Climbing Crew", "Telegram", "group", 0, []obj{omar, sam}, nil)
	msg("climb", "sam", "Sam Okafor", 50*h, "bouldering thursday?", nil)

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
	seed()
	j := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
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
				Text    string `json:"text"`
				ReplyTo string `json:"replyToMessageID"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			extra := obj{}
			if body.ReplyTo != "" {
				extra["linkedMessageID"] = body.ReplyTo
			}
			msg(parts[2], "", "", 0, body.Text, extra)
			bump(parts[2])
			j(w, obj{"chatID": parts[2], "pendingMessageID": "p"})
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
		case p == "/demo/incoming":
			// pretend someone else sent a message: /demo/incoming?chat=hike&from=omar&name=Omar+Haddad&text=hi
			q := r.URL.Query()
			msg(q.Get("chat"), q.Get("from"), q.Get("name"), 0, q.Get("text"), nil)
			bump(q.Get("chat"))
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
			http.ServeFile(w, r, *image)
		case p == "/v1/assets/upload":
			io.Copy(io.Discard, r.Body)
			j(w, obj{"uploadID": "up1", "fileName": "gif.gif", "mimeType": "image/gif"})
		case p == "/v1/accounts":
			j(w, []obj{{"accountID": "x", "network": "WhatsApp", "status": "connected", "user": me}})
		case p == "/v1/assets/download":
			j(w, obj{"srcURL": "file://" + *image})
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
