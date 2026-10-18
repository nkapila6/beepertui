package beeper

import (
	"encoding/json"
	"time"
)

type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	PhoneNumber string `json:"phoneNumber"`
	FullName    string `json:"fullName"`
	Email       string `json:"email"`
	IsSelf      bool   `json:"isSelf"`
}

func (u User) Name() string {
	for _, s := range []string{u.FullName, u.Username, u.PhoneNumber, u.Email, u.ID} {
		if s != "" {
			return s
		}
	}
	return "?"
}

type Participants struct {
	Items   []User `json:"items"`
	HasMore bool   `json:"hasMore"`
	Total   int    `json:"total"`
}

type Chat struct {
	ID             string       `json:"id"`
	AccountID      string       `json:"accountID"`
	Network        string       `json:"network"`
	Title          string       `json:"title"`
	Type           string       `json:"type"`
	IsReadOnly     bool         `json:"isReadOnly"`
	Participants   Participants `json:"participants"`
	LastActivity   time.Time    `json:"lastActivity"`
	UnreadCount    int          `json:"unreadCount"`
	IsMuted        bool         `json:"isMuted"`
	IsPinned       bool         `json:"isPinned"`
	IsArchived     bool         `json:"isArchived"`
	IsLowPriority  bool         `json:"isLowPriority"`
	IsMarkedUnread bool         `json:"isMarkedUnread"`
	LastReadKey    string       `json:"lastReadMessageSortKey"`
	ImgURL         string       `json:"imgURL"`
	Mentions       int          `json:"unreadMentionsCount"`
	Description    string       `json:"description"`
	ExpirySeconds  int          `json:"messageExpirySeconds"`
	Reminder       *Reminder    `json:"reminder"`
	Draft          *Draft       `json:"draft"`
	Labels         []Label      `json:"labels"`
	Preview        *Message     `json:"preview"`
}

type Reminder struct {
	RemindAt                 time.Time `json:"remindAt"`
	DismissOnIncomingMessage bool      `json:"dismissOnIncomingMessage,omitempty"`
}

type Draft struct {
	Text string `json:"text"`
}

type Label struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Account struct {
	AccountID  string `json:"accountID"`
	Network    string `json:"network"`
	User       User   `json:"user"`
	Status     string `json:"status"`
	StatusText string `json:"statusText"`
}

type Upload struct {
	UploadID string `json:"uploadID"`
	FileName string `json:"fileName"`
	MimeType string `json:"mimeType"`
}

type Reaction struct {
	ID            string `json:"id"`
	ReactionKey   string `json:"reactionKey"`
	ParticipantID string `json:"participantID"`
	Emoji         bool   `json:"emoji"`
}

type Attachment struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	SrcURL    string `json:"srcURL"`
	MimeType  string `json:"mimeType"`
	FileName  string `json:"fileName"`
	PosterImg string `json:"posterImg"`
}

type LinkPreview struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type Message struct {
	ID              string        `json:"id"`
	ChatID          string        `json:"chatID"`
	AccountID       string        `json:"accountID"`
	SenderID        string        `json:"senderID"`
	SenderName      string        `json:"senderName"`
	Timestamp       time.Time     `json:"timestamp"`
	SortKey         string        `json:"sortKey"`
	Type            string        `json:"type"`
	Text            string        `json:"text"`
	EditedTimestamp *time.Time    `json:"editedTimestamp"`
	IsSender        bool          `json:"isSender"`
	IsHidden        bool          `json:"isHidden"`
	IsDeleted       bool          `json:"isDeleted"`
	Attachments     []Attachment  `json:"attachments"`
	LinkedMessageID string        `json:"linkedMessageID"`
	Links           []LinkPreview `json:"links"`
	Reactions       []Reaction    `json:"reactions"`
	SendStatus      *SendStatus   `json:"sendStatus"`
	// Seen is a bool, a timestamp, or a map of participant -> either, depending on the network
	Seen json.RawMessage `json:"seen"`
}

type SendStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type page[T any] struct {
	Items        []T    `json:"items"`
	HasMore      bool   `json:"hasMore"`
	OldestCursor string `json:"oldestCursor"`
	NewestCursor string `json:"newestCursor"`
}

type ChatPage = page[Chat]
type MessagePage = page[Message]

type SendResult struct {
	ChatID           string `json:"chatID"`
	PendingMessageID string `json:"pendingMessageID"`
}
