package ui

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Terminals only paste text, so images have to come straight off the system
// clipboard. A copied file (Finder, Files, Explorer) is used as is; raw image
// data (screenshots, "copy image" in a browser) is written to a temp file.
//
//	macOS    osascript («class furl», «class PNGf»)
//	Linux    wl-paste on Wayland, xclip on X11
//	Windows  PowerShell's Windows.Forms clipboard
func clipboardFile(ctx context.Context) (path string, temp bool, err error) {
	switch runtime.GOOS {
	case "darwin":
		return clipboardMac(ctx)
	case "windows":
		return clipboardWindows(ctx)
	}
	return clipboardLinux(ctx)
}

var errNoClipboardImage = errors.New("clipboard has no image or file")

// pasteTarget makes a temp folder holding image.<ext>, so the file goes out
// with a sensible name instead of a temp-file one.
func pasteTarget(ext string) (string, error) {
	dir, err := os.MkdirTemp("", "beepertui-paste-")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "image"+ext), nil
}

// removeTemp deletes a pasted file and the temp folder made for it.
func removeTemp(p string) {
	os.Remove(p)
	if dir := filepath.Dir(p); strings.HasPrefix(filepath.Base(dir), "beepertui-paste-") {
		os.Remove(dir)
	}
}

func clipboardMac(ctx context.Context) (string, bool, error) {
	out, err := exec.CommandContext(ctx, "osascript", "-e", `POSIX path of (the clipboard as «class furl»)`).Output()
	if err == nil {
		if p := strings.TrimSpace(string(out)); isFile(p) {
			return p, false, nil
		}
	}
	dest, err := pasteTarget(".png")
	if err != nil {
		return "", false, err
	}
	script := []string{
		"-e", "on run argv",
		"-e", "set f to open for access (POSIX file (item 1 of argv)) with write permission",
		"-e", "set eof f to 0",
		"-e", "write (the clipboard as «class PNGf») to f",
		"-e", "close access f",
		"-e", "end run",
	}
	if err := exec.CommandContext(ctx, "osascript", append(script, dest)...).Run(); err != nil {
		removeTemp(dest)
		return "", false, errNoClipboardImage
	}
	return dest, true, nil
}

// clipboardLinux asks wl-paste or xclip what's on the clipboard, then takes a
// copied file (text/uri-list) or the best image type on offer.
func clipboardLinux(ctx context.Context) (string, bool, error) {
	var list func() ([]string, error)
	var get func(mime string) ([]byte, error)
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "" && haveCmd("wl-paste"):
		list = func() ([]string, error) {
			out, err := exec.CommandContext(ctx, "wl-paste", "--list-types").Output()
			return strings.Fields(string(out)), err
		}
		get = func(mt string) ([]byte, error) {
			return exec.CommandContext(ctx, "wl-paste", "--no-newline", "--type", mt).Output()
		}
	case haveCmd("xclip"):
		list = func() ([]string, error) {
			out, err := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", "TARGETS", "-o").Output()
			return strings.Fields(string(out)), err
		}
		get = func(mt string) ([]byte, error) {
			return exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", mt, "-o").Output()
		}
	default:
		return "", false, errors.New("pasting images needs wl-clipboard (Wayland) or xclip (X11)")
	}

	types, err := list()
	if err != nil {
		return "", false, errNoClipboardImage
	}
	has := map[string]bool{}
	for _, t := range types {
		has[t] = true
	}
	if has["text/uri-list"] {
		if out, err := get("text/uri-list"); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if p, ok := pastedPath(strings.TrimSpace(line)); ok {
					return p, false, nil
				}
			}
		}
	}
	for _, c := range []struct{ mime, ext string }{
		{"image/png", ".png"}, {"image/jpeg", ".jpg"}, {"image/gif", ".gif"}, {"image/webp", ".webp"},
	} {
		if !has[c.mime] {
			continue
		}
		data, err := get(c.mime)
		if err != nil || len(data) == 0 {
			continue
		}
		dest, err := pasteTarget(c.ext)
		if err != nil {
			return "", false, err
		}
		if err := os.WriteFile(dest, data, 0o600); err != nil {
			removeTemp(dest)
			return "", false, err
		}
		return dest, true, nil
	}
	return "", false, errNoClipboardImage
}

// clipboardWindows uses PowerShell: a copied file wins, else the clipboard image is saved as PNG.
func clipboardWindows(ctx context.Context) (string, bool, error) {
	dest, err := pasteTarget(".png")
	if err != nil {
		return "", false, err
	}
	script := `Add-Type -AssemblyName System.Windows.Forms
$f = [Windows.Forms.Clipboard]::GetFileDropList()
if ($f.Count -gt 0) { "FILE:" + $f[0]; exit }
$i = [Windows.Forms.Clipboard]::GetImage()
if ($i) { $i.Save($env:BEEPERTUI_PASTE, [System.Drawing.Imaging.ImageFormat]::Png); "IMG" }`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", script)
	cmd.Env = append(os.Environ(), "BEEPERTUI_PASTE="+dest)
	out, err := cmd.Output()
	res := strings.TrimSpace(string(out))
	switch {
	case err != nil:
		removeTemp(dest)
		return "", false, errNoClipboardImage
	case strings.HasPrefix(res, "FILE:"):
		removeTemp(dest)
		return strings.TrimPrefix(res, "FILE:"), false, nil
	case res == "IMG" && isFile(dest):
		return dest, true, nil
	}
	removeTemp(dest)
	return "", false, errNoClipboardImage
}

func haveCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
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
		removeTemp(m.attach)
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
