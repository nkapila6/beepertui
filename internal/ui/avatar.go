package ui

import (
	"context"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// Chat avatars: the real image through kitty placeholders where that works,
// otherwise a coloured initials badge. Only chats on screen get loaded.

const (
	avatarCols    = 4
	avatarRows    = 2
	avatarFirstID = 0x5A0000
)

type avatar struct {
	loading bool
	failed  bool
	id      uint32
	png     []byte
}

func (m *Model) loadAvatars() tea.Cmd {
	if !m.showAvatars || m.imageMode != imagesKitty || len(m.chats) == 0 {
		return nil
	}
	first, last := m.chatRange()
	var cmds []tea.Cmd
	for i := first; i <= last && i < len(m.chats); i++ {
		src := m.chats[i].ImgURL
		if src == "" || m.avatars[src] != nil {
			continue
		}
		m.nextImageID++
		a := &avatar{loading: true, id: avatarFirstID + m.nextImageID}
		m.avatars[src] = a
		cmds = append(cmds, m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
			path, err := m.localPath(ctx, src)
			var png []byte
			if err == nil {
				png, _, _, err = shrinkPNG(ctx, path, "96")
			}
			return func(m *Model) tea.Cmd {
				a.loading = false
				if err != nil {
					a.failed = true
					return nil
				}
				a.png = png
				return tea.Raw(kittyTransmit(a.id, png) + kittyPlace(a.id, avatarCols, avatarRows))
			}, nil
		}))
	}
	return tea.Batch(cmds...)
}

func (m *Model) avatarResend() string {
	var b strings.Builder
	for _, a := range m.avatars {
		if len(a.png) > 0 {
			b.WriteString(kittyTransmit(a.id, a.png))
			b.WriteString(kittyPlace(a.id, avatarCols, avatarRows))
		}
	}
	return b.String()
}

// avatarLines is the two lines of a chat's avatar.
func (m *Model) avatarLines(ch beeper.Chat) [2]string {
	if a := m.avatars[ch.ImgURL]; a != nil && len(a.png) > 0 {
		l := kittyLines(a.id, avatarCols, avatarRows)
		return [2]string{l[0], l[1]}
	}
	c := hashColor(ch.ID)
	if m.modern {
		// coloured initials on a faint tile, quieter than a solid block
		tile := lipgloss.NewStyle().Foreground(c).Background(chipBg()).Bold(true)
		return [2]string{tile.Render(padCenter(initials(ch.Title), avatarCols)), strings.Repeat(" ", avatarCols)}
	}
	badge := pill(c)
	// the second line is a half block in the same colour, so the badge reads as one rounded-ish tile
	return [2]string{
		badge.Render(padCenter(initials(ch.Title), avatarCols)),
		lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("▀", avatarCols)),
	}
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		for _, r := range w {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, unicode.ToUpper(r))
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "#"
	}
	return string(out)
}

func padCenter(s string, w int) string {
	gap := max(0, w-lipgloss.Width(s))
	return strings.Repeat(" ", gap/2) + s + strings.Repeat(" ", gap-gap/2)
}
