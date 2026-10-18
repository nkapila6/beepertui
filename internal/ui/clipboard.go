package ui

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Terminals only paste text, so images have to come straight off the macOS
// pasteboard. A copied Finder file is used as is; raw image data (screenshots,
// "copy image" in a browser) is written out to a temp PNG.
func clipboardFile(ctx context.Context) (path string, temp bool, err error) {
	out, err := exec.CommandContext(ctx, "osascript", "-e", `POSIX path of (the clipboard as «class furl»)`).Output()
	if err == nil {
		if p := strings.TrimSpace(string(out)); isFile(p) {
			return p, false, nil
		}
	}

	f, err := os.CreateTemp("", "beepertui-paste-*.png")
	if err != nil {
		return "", false, err
	}
	f.Close()
	script := []string{
		"-e", "on run argv",
		"-e", "set f to open for access (POSIX file (item 1 of argv)) with write permission",
		"-e", "set eof f to 0",
		"-e", "write (the clipboard as «class PNGf») to f",
		"-e", "close access f",
		"-e", "end run",
	}
	if err := exec.CommandContext(ctx, "osascript", append(script, f.Name())...).Run(); err != nil {
		os.Remove(f.Name())
		return "", false, errors.New("clipboard has no image or file")
	}
	return f.Name(), true, nil
}

func (m *Model) pasteFromClipboard() tea.Cmd {
	if m.cur == nil {
		m.setErr("open a chat first")
		return nil
	}
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		p, temp, err := clipboardFile(ctx)
		if err != nil {
			return nil, err
		}
		return func(m *Model) tea.Cmd {
			m.setAttach(p, temp)
			m.setStatus("attached from clipboard, enter sends")
			return nil
		}, nil
	})
}

// setAttach stages a file for the next message, cleaning up any temp file it replaces.
func (m *Model) setAttach(path string, temp bool) {
	if m.attachTemp && m.attach != "" && m.attach != path {
		os.Remove(m.attach)
	}
	m.attach, m.attachTemp = path, temp
}

// pastedPath turns a dragged-in file (which the terminal pastes as a path,
// often quoted or with escaped spaces) back into a real path.
func pastedPath(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, "\n\r") || s == "" {
		return "", false
	}
	s = strings.Trim(s, `'"`)
	if strings.HasPrefix(s, "file://") {
		if u, err := url.Parse(s); err == nil {
			s = u.Path
		}
	}
	s = strings.ReplaceAll(s, `\ `, " ")
	s = expandHome(s)
	if !strings.HasPrefix(s, "/") || !isFile(s) {
		return "", false
	}
	return s, true
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
