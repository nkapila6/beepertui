package ui

import (
	"context"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
	"github.com/nkapila6/beepertui/internal/config"
)

type pane int

const (
	paneChats pane = iota
	paneMsgs
)

type mode int

const (
	modeNormal mode = iota
	modeInsert
	modeLine // bottom prompt: search, command, react, generic text
	modeFlash
	modeConfirm
	modePicker
	modeHelp
	modePopup
	modeLinks
	modeMenu
	modeGif
)

// pollEvery backstops bridges that never emit websocket events (iMessage, as of 2026).
const pollEvery = 30 * time.Second

var inboxes = []string{"primary", "low-priority", "archive", "unread"}

type Model struct {
	c      *beeper.Client
	events chan beeper.Event
	cancel context.CancelFunc

	w, h  int
	focus pane
	mode  mode

	// inbox
	chats      []beeper.Chat
	chatCur    int
	chatOff    int
	inbox      int
	label      beeper.Label
	query      string
	reloadChat bool

	// open chat
	cur        *beeper.Chat
	msgs       []beeper.Message
	msgCur     int
	msgBottom  int
	oldest     string
	hasMore    bool
	reloadMsgs bool
	jumpTo     string
	jumpTries  int
	selfIDs    map[string]bool

	// compose
	compose textarea.Model
	replyTo *beeper.Message
	editing *beeper.Message
	attach  string
	// attachTemp marks a clipboard image we wrote ourselves and should delete
	attachTemp bool

	// bottom prompt
	line       textinput.Model
	lineKind   string
	lineSubmit func(*Model, string) tea.Cmd
	lineKey    func(*Model, string) (bool, tea.Cmd)
	paletteSel int

	// flash
	flash flashState

	recentReacts []string
	thumbs       map[string]*thumb
	showImages   bool
	imageMode    imageMode
	nextImageID  uint32
	cellAspect   float64
	popup        *popup
	mouse        bool

	mention          *mentionState
	emoji            *emojiState
	mentioned        map[string]string // "@Name" -> user id, for the draft being written
	mentionDismissed int

	toast     *toast
	toastSeq  int
	notifySeq int

	focused      bool           // terminal has focus, from focus reporting
	seenUnread   map[string]int // last unread count per chat, to spot new messages
	notifPreview bool

	nerdFont bool
	compact  bool
	spinning bool
	spin     int
	readMark string // last read sort key when the chat was opened, for the "new" line

	resizeSeq int

	cfg           *config.Config
	chatPct       int
	hideChats     bool // ctrl+" / ctrl+b, only while a chat is open
	notifications bool

	hints     linkHints
	hover     *hoverState // reaction row under the mouse
	menu      *ctxMenu
	gif       *gifPicker
	mediaTab  string // last picker tab, reopened by ctrl+g
	animating bool   // the GIF animation ticker is running

	avatars     map[string]*avatar
	showAvatars bool
	helpOff     int

	// confirm
	confirmText string
	confirmYes  func(*Model) tea.Cmd

	// picker
	pick picker

	pendingKey string // first half of a two-key bind like "g g"
	km         keymap
	capture    keyCapture // :keys waiting for a key press
	keysNote   string     // last change, shown in the :keys footer
	vim        vimState   // compose box normal mode (vim_compose)
	vimCompose bool
	live       bool
	inflight   int
	status     string
	isErr      bool
}

func New(c *beeper.Client, cfg *config.Config) *Model {
	ci := newCompose()

	li := textinput.New()
	li.CharLimit = 0

	ctx, cancel := context.WithCancel(context.Background())
	m := &Model{
		c:          c,
		events:     make(chan beeper.Event, 64),
		cancel:     cancel,
		compose:    ci,
		line:       li,
		selfIDs:    map[string]bool{},
		thumbs:     map[string]*thumb{},
		showImages: true,
		cellAspect: cellAspect(),

		mentionDismissed: -1,
		avatars:          map[string]*avatar{},
		cfg:              cfg,
		focused:          true,
		seenUnread:       map[string]int{},
	}
	for _, st := range config.Settings {
		m.applySetting(st.Key)
	}
	m.km = buildKeymap(cfg)
	go c.Stream(ctx, m.events)
	return m
}

type (
	doneMsg struct {
		status string
		err    error
		then   func(*Model) tea.Cmd
	}
	eventMsg  beeper.Event
	pollMsg   struct{}
	reloadMsg struct{ chats, msgs bool }
)

// run does fn off the UI goroutine. fn must not touch the model; put state changes in the returned func.
func (m *Model) run(status string, fn func(ctx context.Context) (func(*Model) tea.Cmd, error)) tea.Cmd {
	m.inflight++
	work := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		then, err := fn(ctx)
		return doneMsg{status: status, err: err, then: then}
	}
	if !m.spinning {
		m.spinning = true
		return tea.Batch(work, spinTick())
	}
	return work
}

func (m *Model) waitEvent() tea.Cmd {
	return func() tea.Msg { return eventMsg(<-m.events) }
}

func poll() tea.Cmd {
	return tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} })
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.loadChats(), m.loadSelf(), m.waitEvent(), poll())
}

func (m *Model) setStatus(s string) { m.status, m.isErr = s, false }
func (m *Model) setErr(s string)    { m.status, m.isErr = s, true }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.cellAspect = cellAspect()
		m.compose.SetWidth(max(10, m.msgWidth()-4))
		m.line.SetWidth(max(10, m.w-20))
		m.fixChatScroll()
		m.fixMsgScroll()
		return m, tea.Batch(m.placeImages(), m.placePopup(), m.placeGifPreview(), m.afterResize())

	case videoDoneMsg:
		return m, m.afterVideo(msg.err)

	case animMsg:
		return m, m.stepAnim()

	case memeCaptionMsg:
		if m.gif != nil && m.gif.seq == msg.seq {
			return m, m.refreshMemePreview()
		}
		return m, nil

	case gifSearchMsg:
		if m.gif != nil && m.gif.seq == msg.seq {
			return m, m.searchGifs()
		}
		return m, nil

	case resizedMsg:
		if msg.seq != m.resizeSeq {
			return m, nil // still resizing
		}
		return m, m.resendImages()

	case doneMsg:
		m.inflight = max(0, m.inflight-1)
		if msg.err != nil {
			m.setErr(friendlyErr(msg.err))
			return m, nil
		}
		if msg.status != "" {
			m.setStatus(msg.status)
		}
		if msg.then != nil {
			return m, msg.then(m)
		}
		return m, nil

	case eventMsg:
		m.live = true
		return m, tea.Batch(m.waitEvent(), m.onEvent(beeper.Event(msg)))

	case pollMsg:
		return m, tea.Batch(poll(), m.loadChats(), m.reloadOpenChat())

	case reloadMsg:
		var cmds []tea.Cmd
		if msg.chats {
			m.reloadChat = false
			cmds = append(cmds, m.loadChats())
		}
		if msg.msgs {
			m.reloadMsgs = false
			cmds = append(cmds, m.reloadOpenChat())
		}
		return m, tea.Batch(cmds...)

	case tea.KeyPressMsg:
		return m, tea.Batch(m.handleKey(msg), m.loadAvatars())

	case spinMsg:
		m.spin++
		if m.inflight > 0 {
			return m, spinTick()
		}
		m.spinning = false
		return m, nil

	case tea.FocusMsg:
		m.focused = true
		return m, nil

	case tea.BlurMsg:
		m.focused = false
		return m, nil

	case toastDoneMsg:
		if m.toast != nil && m.toast.seq == msg.seq {
			m.toast = nil
		}
		return m, nil

	case tea.MouseMsg:
		return m, tea.Batch(m.handleMouse(msg), m.loadAvatars())

	case tea.PasteMsg:
		// a file dragged into the terminal arrives as its path
		if m.mode == modeInsert {
			if p, ok := pastedPath(msg.Content); ok {
				m.setAttach(p, false)
				m.setStatus("attached " + p + ", enter sends")
				return m, nil
			}
		}
	}

	// cursor blink and friends
	var cmd tea.Cmd
	switch m.mode {
	case modeInsert:
		m.compose, cmd = m.compose.Update(msg)
	case modeLine, modePicker, modeGif:
		m.line, cmd = m.line.Update(msg)
	}
	return m, cmd
}

// onEvent debounces refetches, since a busy group can fire dozens of events a second.
func (m *Model) onEvent(ev beeper.Event) tea.Cmd {
	wantChats := !m.reloadChat
	wantMsgs := false
	switch ev.Type {
	case beeper.EventReconnected:
		wantMsgs = m.cur != nil
	case "message.upserted", "message.deleted":
		wantMsgs = m.cur != nil && ev.ChatID == m.cur.ID
	}
	wantMsgs = wantMsgs && !m.reloadMsgs
	if !wantChats && !wantMsgs {
		return nil
	}
	m.reloadChat = m.reloadChat || wantChats
	m.reloadMsgs = m.reloadMsgs || wantMsgs
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return reloadMsg{chats: wantChats, msgs: wantMsgs}
	})
}

func (m *Model) filter() beeper.InboxFilter {
	f := beeper.InboxFilter{Query: m.query, LabelID: m.label.ID}
	if inboxes[m.inbox] == "unread" {
		f.UnreadOnly = true
	} else {
		f.Inbox = inboxes[m.inbox]
	}
	return f
}

func (m *Model) loadChats() tea.Cmd {
	f := m.filter()
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		p, err := m.c.FilterChats(ctx, f, 100)
		if err != nil {
			return nil, err
		}
		// search results come without the last message, only the plain list has it
		if recent, err := m.c.ListChats(ctx, 200); err == nil {
			prev := make(map[string]*beeper.Message, len(recent.Items))
			for _, ch := range recent.Items {
				prev[ch.ID] = ch.Preview
			}
			for i := range p.Items {
				if p.Items[i].Preview == nil {
					p.Items[i].Preview = prev[p.Items[i].ID]
				}
			}
		}
		return func(m *Model) tea.Cmd {
			var selID string
			if m.chatCur < len(m.chats) {
				selID = m.chats[m.chatCur].ID
			}
			alert := m.notifyNewMessages(p.Items)
			m.chats = p.Items
			m.chatCur = 0
			for i, ch := range m.chats {
				if ch.ID == selID {
					m.chatCur = i
				}
				if m.cur != nil && ch.ID == m.cur.ID {
					c := ch
					m.cur = &c
				}
			}
			m.fixChatScroll()
			return tea.Batch(alert, m.loadAvatars())
		}, nil
	})
}

func (m *Model) loadSelf() tea.Cmd {
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		accts, err := m.c.Accounts(ctx)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			for _, a := range accts {
				m.selfIDs[a.User.ID] = true
			}
			return nil
		}, nil
	})
}

func (m *Model) openChatByID(id string, jumpTo string) tea.Cmd {
	for _, ch := range m.chats {
		if ch.ID == id {
			return m.openChat(ch, jumpTo)
		}
	}
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		ch, err := m.c.GetChat(ctx, id)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd { return m.openChat(ch, jumpTo) }, nil
	})
}

func (m *Model) openChat(ch beeper.Chat, jumpTo string) tea.Cmd {
	if m.cur == nil || m.cur.ID != ch.ID {
		m.msgs = nil
		m.msgCur, m.msgBottom = 0, 0
		m.replyTo, m.editing = nil, nil
		m.setAttach("", false)
		m.compose.SetValue("")
		m.mention, m.mentioned = nil, nil
		if ch.Draft != nil {
			m.compose.SetValue(ch.Draft.Text)
		}
	}
	if m.cur == nil || m.cur.ID != ch.ID {
		m.readMark = ""
		if ch.UnreadCount > 0 {
			m.readMark = ch.LastReadKey
		}
	}
	m.cur = &ch
	m.focus = paneMsgs
	m.jumpTo, m.jumpTries = jumpTo, 0
	for _, p := range ch.Participants.Items {
		if p.IsSelf {
			m.selfIDs[p.ID] = true
		}
	}
	cmds := []tea.Cmd{m.reloadOpenChat()}
	if ch.UnreadCount > 0 || ch.IsMarkedUnread {
		id := ch.ID
		cmds = append(cmds, m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			return nil, m.c.MarkRead(ctx, id)
		}))
	}
	return tea.Batch(cmds...)
}

func (m *Model) reloadOpenChat() tea.Cmd {
	if m.cur == nil {
		return nil
	}
	id := m.cur.ID
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		p, err := m.c.ListMessages(ctx, id, "")
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			if m.cur == nil || m.cur.ID != id {
				return nil
			}
			// follow new messages when you're at the end: cursor on the last
			// one, the last one on screen, or writing
			n := len(m.msgs)
			atBottom := n == 0 || m.msgCur >= n-1 || m.msgBottom >= n-1 || m.mode == modeInsert
			var selID string
			if m.msgCur < len(m.msgs) {
				selID = m.msgs[m.msgCur].ID
			}
			// keep any older history we already paged in
			fresh := visible(p.Items)
			merged := false
			if len(m.msgs) > 0 && len(fresh) > 0 {
				first := fresh[0]
				for i, old := range m.msgs {
					if !older(old, first) {
						fresh = append(slices.Clone(m.msgs[:i]), fresh...)
						merged = true
						break
					}
				}
			}
			if !merged {
				m.oldest, m.hasMore = p.OldestCursor, p.HasMore
			}
			m.msgs = fresh
			m.msgCur = len(m.msgs) - 1
			if !atBottom {
				for i, mm := range m.msgs {
					if mm.ID == selID {
						m.msgCur = i
					}
				}
			}
			m.msgCur = max(0, m.msgCur)
			return m.afterMsgsLoaded()
		}, nil
	})
}

func (m *Model) loadOlder() tea.Cmd {
	if m.cur == nil || !m.hasMore || m.oldest == "" {
		return nil
	}
	id, cursor := m.cur.ID, m.oldest
	return m.run("loaded older messages", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		p, err := m.c.ListMessages(ctx, id, cursor)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			if m.cur == nil || m.cur.ID != id {
				return nil
			}
			add := visible(p.Items)
			m.msgs = append(add, m.msgs...)
			m.msgCur += len(add)
			m.msgBottom += len(add)
			m.oldest, m.hasMore = p.OldestCursor, p.HasMore
			return m.afterMsgsLoaded()
		}, nil
	})
}

func (m *Model) afterMsgsLoaded() tea.Cmd {
	if m.jumpTo != "" {
		for i, mm := range m.msgs {
			if mm.ID == m.jumpTo {
				m.msgCur = i
				m.jumpTo = ""
				break
			}
		}
		if m.jumpTo != "" {
			if m.hasMore && m.jumpTries < 10 {
				m.jumpTries++
				m.fixMsgScroll()
				return m.loadOlder()
			}
			m.jumpTo = ""
			m.setErr("couldn't find that message in history")
		}
	}
	m.fixMsgScroll()
	return m.loadThumbs()
}

func older(a, b beeper.Message) bool {
	if !a.Timestamp.Equal(b.Timestamp) {
		return a.Timestamp.Before(b.Timestamp)
	}
	return a.SortKey < b.SortKey
}

// visible drops reaction events and hidden rows, and sorts oldest first.
func visible(in []beeper.Message) []beeper.Message {
	out := make([]beeper.Message, 0, len(in))
	for _, mm := range in {
		if mm.IsHidden || mm.Type == "REACTION" {
			continue
		}
		out = append(out, mm)
	}
	slices.SortStableFunc(out, func(a, b beeper.Message) int {
		if older(a, b) {
			return -1
		}
		if older(b, a) {
			return 1
		}
		return 0
	})
	return out
}

func friendlyErr(err error) string {
	s := err.Error()
	if strings.Contains(s, "connection refused") {
		return "can't reach Beeper Desktop. Is it running with the API enabled?"
	}
	if ae, ok := err.(*beeper.APIError); ok && ae.Status == 401 {
		return "token rejected (401). Run beepertui login again"
	}
	return s
}

func (m *Model) Close() { m.cancel() }
