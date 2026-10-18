package beeper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "http://localhost:23373"

type Client struct {
	BaseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("beeper api %d: %s", e.Status, e.Body)
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return &APIError{Status: resp.StatusCode, Body: string(bytes.TrimSpace(b))}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) ListChats(ctx context.Context, limit int) (ChatPage, error) {
	var p ChatPage
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	err := c.do(ctx, http.MethodGet, "/v1/chats", q, nil, &p)
	return p, err
}

func (c *Client) SearchChats(ctx context.Context, query string, limit int) (ChatPage, error) {
	var p ChatPage
	q := url.Values{"query": {query}, "limit": {strconv.Itoa(limit)}}
	err := c.do(ctx, http.MethodGet, "/v1/chats/search", q, nil, &p)
	return p, err
}

// ListMessages fetches the newest page when cursor is empty, otherwise older messages before cursor.
func (c *Client) ListMessages(ctx context.Context, chatID, cursor string) (MessagePage, error) {
	var p MessagePage
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
		q.Set("direction", "before")
	}
	err := c.do(ctx, http.MethodGet, "/v1/chats/"+url.PathEscape(chatID)+"/messages", q, nil, &p)
	return p, err
}

func (c *Client) SendMessage(ctx context.Context, chatID, text, replyTo string) (SendResult, error) {
	var r SendResult
	body := map[string]string{"text": text}
	if replyTo != "" {
		body["replyToMessageID"] = replyTo
	}
	err := c.do(ctx, http.MethodPost, "/v1/chats/"+url.PathEscape(chatID)+"/messages", nil, body, &r)
	return r, err
}

func (c *Client) EditMessage(ctx context.Context, chatID, msgID, text string) error {
	p := "/v1/chats/" + url.PathEscape(chatID) + "/messages/" + url.PathEscape(msgID)
	return c.do(ctx, http.MethodPut, p, nil, map[string]string{"text": text}, nil)
}

func (c *Client) DeleteMessage(ctx context.Context, chatID, msgID string) error {
	p := "/v1/chats/" + url.PathEscape(chatID) + "/messages/" + url.PathEscape(msgID)
	return c.do(ctx, http.MethodDelete, p, nil, nil, nil)
}

func (c *Client) AddReaction(ctx context.Context, chatID, msgID, key string) error {
	p := "/v1/chats/" + url.PathEscape(chatID) + "/messages/" + url.PathEscape(msgID) + "/reactions"
	return c.do(ctx, http.MethodPost, p, nil, map[string]string{"reactionKey": key}, nil)
}

func (c *Client) MarkRead(ctx context.Context, chatID string) error {
	return c.do(ctx, http.MethodPost, "/v1/chats/"+url.PathEscape(chatID)+"/read", nil, map[string]string{}, nil)
}

func chatPath(chatID string) string { return "/v1/chats/" + url.PathEscape(chatID) }

func msgPath(chatID, msgID string) string {
	return chatPath(chatID) + "/messages/" + url.PathEscape(msgID)
}

type InboxFilter struct {
	Inbox      string // primary, low-priority, archive
	UnreadOnly bool
	LabelID    string
	Query      string
}

// FilterChats uses the search endpoint since it's the only one that understands inboxes and labels.
func (c *Client) FilterChats(ctx context.Context, f InboxFilter, limit int) (ChatPage, error) {
	var p ChatPage
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if f.Inbox != "" {
		q.Set("inbox", f.Inbox)
	}
	if f.UnreadOnly {
		q.Set("unreadOnly", "true")
	}
	if f.LabelID != "" {
		q.Set("labelID", f.LabelID)
	}
	if f.Query != "" {
		q.Set("query", f.Query)
	}
	err := c.do(ctx, http.MethodGet, "/v1/chats/search", q, nil, &p)
	return p, err
}

func (c *Client) GetChat(ctx context.Context, chatID string) (Chat, error) {
	var ch Chat
	err := c.do(ctx, http.MethodGet, chatPath(chatID), nil, nil, &ch)
	return ch, err
}

// UpdateChat sends a partial PATCH, so only set the keys you want changed.
func (c *Client) UpdateChat(ctx context.Context, chatID string, fields map[string]any) error {
	return c.do(ctx, http.MethodPatch, chatPath(chatID), nil, fields, nil)
}

func (c *Client) Archive(ctx context.Context, chatID string, archived bool) error {
	return c.do(ctx, http.MethodPost, chatPath(chatID)+"/archive", nil, map[string]bool{"archived": archived}, nil)
}

func (c *Client) MarkUnread(ctx context.Context, chatID string) error {
	return c.do(ctx, http.MethodPost, chatPath(chatID)+"/unread", nil, map[string]string{}, nil)
}

func (c *Client) NotifyAnyway(ctx context.Context, chatID string) error {
	return c.do(ctx, http.MethodPost, chatPath(chatID)+"/notify-anyway", nil, map[string]string{}, nil)
}

func (c *Client) SetReminder(ctx context.Context, chatID string, at time.Time, dismissOnIncoming bool) error {
	body := map[string]Reminder{"reminder": {RemindAt: at.UTC(), DismissOnIncomingMessage: dismissOnIncoming}}
	return c.do(ctx, http.MethodPost, chatPath(chatID)+"/reminders", nil, body, nil)
}

func (c *Client) ClearReminder(ctx context.Context, chatID string) error {
	return c.do(ctx, http.MethodDelete, chatPath(chatID)+"/reminders", nil, nil, nil)
}

func (c *Client) RemoveReaction(ctx context.Context, chatID, msgID, key string) error {
	return c.do(ctx, http.MethodDelete, msgPath(chatID, msgID)+"/reactions/"+url.PathEscape(key), nil, nil, nil)
}

func (c *Client) SendAttachment(ctx context.Context, chatID string, up Upload, caption, replyTo string) (SendResult, error) {
	var r SendResult
	att := map[string]any{"uploadID": up.UploadID}
	if up.MimeType != "" {
		att["mimeType"] = up.MimeType
		att["type"] = attachmentType(up.MimeType)
	}
	if up.FileName != "" {
		att["fileName"] = up.FileName
	}
	if up.Width > 0 && up.Height > 0 {
		att["size"] = map[string]float64{"width": up.Width, "height": up.Height}
	}
	body := map[string]any{"attachment": att}
	if caption != "" {
		body["text"] = caption
	}
	if replyTo != "" {
		body["replyToMessageID"] = replyTo
	}
	err := c.do(ctx, http.MethodPost, chatPath(chatID)+"/messages", nil, body, &r)
	return r, err
}

func (c *Client) Upload(ctx context.Context, path string) (Upload, error) {
	var up Upload
	f, err := os.Open(path)
	if err != nil {
		return up, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return up, err
	}
	// Beeper decides image vs gif vs plain file from the mime type, and
	// CreateFormFile would label everything application/octet-stream
	mt := detectMime(path, data)
	name := filepath.Base(path)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, name))
	h.Set("Content-Type", mt)
	fw, err := mw.CreatePart(h)
	if err != nil {
		return up, err
	}
	if _, err := fw.Write(data); err != nil {
		return up, err
	}
	mw.WriteField("fileName", name)
	mw.WriteField("mimeType", mt)
	if err := mw.Close(); err != nil {
		return up, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/assets/upload", &buf)
	if err != nil {
		return up, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.http.Do(req)
	if err != nil {
		return up, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return up, &APIError{Status: resp.StatusCode, Body: string(bytes.TrimSpace(b))}
	}
	return up, json.NewDecoder(resp.Body).Decode(&up)
}

// Download resolves an mxc:// attachment URL to a local file:// URL.
func (c *Client) Download(ctx context.Context, srcURL string) (string, error) {
	var out struct {
		SrcURL string `json:"srcURL"`
		Error  string `json:"error"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/assets/download", nil, map[string]string{"url": srcURL}, &out); err != nil {
		return "", err
	}
	if out.Error != "" {
		return "", fmt.Errorf("download: %s", out.Error)
	}
	return out.SrcURL, nil
}

func (c *Client) SearchMessages(ctx context.Context, query string, limit int) (MessagePage, error) {
	var p MessagePage
	q := url.Values{"query": {query}, "limit": {strconv.Itoa(limit)}}
	err := c.do(ctx, http.MethodGet, "/v1/messages/search", q, nil, &p)
	return p, err
}

func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var out []Account
	err := c.do(ctx, http.MethodGet, "/v1/accounts", nil, nil, &out)
	return out, err
}

func (c *Client) Labels(ctx context.Context) ([]Label, error) {
	var out []Label
	err := c.do(ctx, http.MethodGet, "/v1/labels", nil, nil, &out)
	return out, err
}

func (c *Client) SearchContacts(ctx context.Context, accountID, query string) ([]User, error) {
	var out struct {
		Items []User `json:"items"`
	}
	q := url.Values{"query": {query}}
	err := c.do(ctx, http.MethodGet, "/v1/accounts/"+url.PathEscape(accountID)+"/contacts", q, nil, &out)
	return out.Items, err
}

// StartDirect resolves a user (by id, handle, phone or email) and opens or creates the DM.
func (c *Client) StartDirect(ctx context.Context, accountID string, u User) (string, error) {
	var out Chat
	who := map[string]string{}
	for k, v := range map[string]string{"id": u.ID, "username": u.Username, "phoneNumber": u.PhoneNumber, "email": u.Email} {
		if v != "" {
			who[k] = v
		}
	}
	body := map[string]any{"accountID": accountID, "user": who}
	err := c.do(ctx, http.MethodPost, "/v1/chats/start", nil, body, &out)
	return out.ID, err
}

func (c *Client) CreateGroup(ctx context.Context, accountID, title string, participantIDs []string) (string, error) {
	var out Chat
	body := map[string]any{"accountID": accountID, "type": "group", "title": title, "participantIDs": participantIDs}
	err := c.do(ctx, http.MethodPost, "/v1/chats", nil, body, &out)
	return out.ID, err
}

// Focus brings Beeper Desktop to the front, optionally on a chat or message.
func (c *Client) Focus(ctx context.Context, chatID, msgID string) error {
	body := map[string]string{}
	if chatID != "" {
		body["chatID"] = chatID
	}
	if msgID != "" {
		body["messageID"] = msgID
	}
	return c.do(ctx, http.MethodPost, "/v1/focus", nil, body, nil)
}

// detectMime sniffs the content, falling back to the extension, so a pasted
// screenshot goes out as image/png rather than a generic file.
func detectMime(path string, data []byte) string {
	mt := http.DetectContentType(data)
	if mt == "application/octet-stream" || strings.HasPrefix(mt, "text/plain") {
		if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); byExt != "" {
			return byExt
		}
	}
	if i := strings.Index(mt, ";"); i >= 0 && !strings.HasPrefix(mt, "text/") {
		mt = mt[:i]
	}
	return mt
}

// attachmentType maps a mime type to Beeper's attachment type hint.
func attachmentType(mt string) string {
	switch {
	case mt == "image/gif":
		return "gif"
	case strings.HasPrefix(mt, "image/"):
		return "image"
	case strings.HasPrefix(mt, "video/"):
		return "video"
	case strings.HasPrefix(mt, "audio/"):
		return "audio"
	}
	return "file"
}
