package ui

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// openURL hands a URL or file:// path to the OS default app.
func openURL(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Run()
	}
	return exec.Command("xdg-open", u).Run()
}

// reveal shows a file in the file manager without opening it.
func reveal(fileURL string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", fileURL).Run()
	}
	// no portable "select this file", so open the folder it's in
	return openURL("file://" + filepath.Dir(strings.TrimPrefix(fileURL, "file://")))
}

// copyText puts text on the system clipboard. Callers fall back to OSC52 on error.
func copyText(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	default:
		for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "-ib"}} {
			if _, err := exec.LookPath(c[0]); err == nil {
				cmd = exec.Command(c[0], c[1:]...)
				break
			}
		}
		if cmd == nil {
			return errors.New("no clipboard tool")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

var errPasteUnsupported = errors.New("pasting images is macOS only for now, use :attach <path>")

func pasteSupported(context.Context) error {
	if runtime.GOOS != "darwin" {
		return errPasteUnsupported
	}
	return nil
}
