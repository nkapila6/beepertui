package beeper

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type Event struct {
	Type   string   `json:"type"`
	ChatID string   `json:"chatID"`
	IDs    []string `json:"ids"`
}

// EventReconnected is synthetic. Delivery is at-most-once with no replay, so
// the UI should refetch whatever it shows when it sees this.
const EventReconnected = "reconnected"

// Stream keeps a websocket open to /v1/ws and pushes events until ctx ends.
// It reconnects with backoff on its own.
func (c *Client) Stream(ctx context.Context, out chan<- Event) {
	backoff := time.Second
	first := true
	for ctx.Err() == nil {
		err := c.streamOnce(ctx, out, !first)
		if ctx.Err() != nil {
			return
		}
		first = false
		if err == nil {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (c *Client) streamOnce(ctx context.Context, out chan<- Event, reconnect bool) error {
	wsURL := "ws" + strings.TrimPrefix(c.BaseURL, "http") + "/v1/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + c.token}},
	})
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(8 << 20)

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var ev Event
		if json.Unmarshal(data, &ev) != nil {
			continue
		}
		switch ev.Type {
		case "ready":
			sub := []byte(`{"type":"subscriptions.set","chatIDs":["*"]}`)
			if err := conn.Write(ctx, websocket.MessageText, sub); err != nil {
				return err
			}
			if reconnect {
				out <- Event{Type: EventReconnected}
			}
		case "chat.upserted", "chat.deleted", "message.upserted", "message.deleted":
			select {
			case out <- ev:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}
