package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Attaching files without typing paths: the native Finder dialog on macOS,
// and a small folder browser in the TUI everywhere (:files).

var errCancelled = errors.New("cancelled")

// chooseFile picks an attachment the friendliest way the platform allows.
func (m *Model) chooseFile() tea.Cmd {
	if m.cur == nil {
		m.setErr("open a chat first")
		return nil
	}
	dialog := nativeDialog()
	if dialog == nil {
		return m.browseFiles("")
	}
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		path, err := dialog(ctx)
		return func(m *Model) tea.Cmd {
			switch {
			case errors.Is(err, errCancelled):
				m.setStatus("no file picked")
			case err != nil:
				m.setErr(err.Error())
			default:
				m.setAttach(path, false)
				m.setStatus("attached " + filepath.Base(path) + ", enter sends")
			}
			return nil
		}, nil
	})
}

// nativeDialog is the desktop's own file picker if there is one: Finder on
// macOS, zenity (GNOME) or kdialog (KDE) on Linux. nil means use the TUI browser.
func nativeDialog() func(context.Context) (string, error) {
	switch {
	case runtime.GOOS == "darwin":
		return finderDialog
	case runtime.GOOS != "linux" || os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "":
		return nil
	case haveCmd("zenity"):
		return func(ctx context.Context) (string, error) {
			return runDialog(exec.CommandContext(ctx, "zenity", "--file-selection", "--title=Attach a file to send with beepertui"))
		}
	case haveCmd("kdialog"):
		return func(ctx context.Context) (string, error) {
			home, _ := os.UserHomeDir()
			return runDialog(exec.CommandContext(ctx, "kdialog", "--getopenfilename", home))
		}
	}
	return nil
}

// runDialog runs zenity/kdialog; both exit 1 when you cancel.
func runDialog(cmd *exec.Cmd) (string, error) {
	out, err := cmd.Output()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return "", errCancelled
	}
	if err != nil {
		return "", fmt.Errorf("file dialog: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func finderDialog(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "osascript",
		"-e", `POSIX path of (choose file with prompt "Attach a file to send with beepertui")`).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && strings.Contains(string(ee.Stderr), "-128") {
			return "", errCancelled
		}
		return "", fmt.Errorf("file dialog: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

type fileEntry struct {
	path string
	dir  bool
}

// browseFiles lists a folder in the picker. Folders open, files attach.
func (m *Model) browseFiles(dir string) tea.Cmd {
	if m.cur == nil {
		m.setErr("open a chat first")
		return nil
	}
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Downloads")
		if _, err := os.Stat(dir); err != nil {
			dir = home
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.setErr(err.Error())
		return nil
	}
	sort.SliceStable(entries, func(a, b int) bool {
		if entries[a].IsDir() != entries[b].IsDir() {
			return entries[a].IsDir()
		}
		return strings.ToLower(entries[a].Name()) < strings.ToLower(entries[b].Name())
	})
	items := []pickItem{{title: "..", detail: "up a folder", value: fileEntry{filepath.Dir(dir), true}}}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			items = append(items, pickItem{title: e.Name() + "/", value: fileEntry{p, true}})
			continue
		}
		detail := ""
		if info, err := e.Info(); err == nil {
			detail = humanSize(info.Size())
		}
		items = append(items, pickItem{title: e.Name(), detail: detail, value: fileEntry{p, false}})
	}
	title := strings.Replace(dir, os.Getenv("HOME"), "~", 1)
	m.openPicker("attach from "+title, items, func(m *Model, it pickItem) tea.Cmd {
		f := it.value.(fileEntry)
		if f.dir {
			return m.browseFiles(f.path)
		}
		m.setAttach(f.path, false)
		m.setStatus("attached " + filepath.Base(f.path) + ", enter sends")
		return m.startInsert()
	})
	return nil
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
