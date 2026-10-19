package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nkapila6/beepertui/internal/beeper"
)

// Videos. Beeper often sends no poster frame, so with ffmpeg installed we
// grab one from the video itself. Playing hands the terminal to mpv, which
// draws in kitty/Ghostty with the kitty protocol (text art elsewhere) and has
// sound. Both are optional: without ffmpeg there's just the label, without
// mpv the video opens in the default app.

const videoPrefix = "video:" // thumb key for a poster we make ourselves

func isVideo(a beeper.Attachment) bool {
	return a.Type == "video" || strings.HasPrefix(a.MimeType, "video/")
}

// videoPoster writes one frame of the video to a temp PNG. The caller removes it.
func videoPoster(ctx context.Context, path string) (string, time.Duration, error) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", 0, fmt.Errorf("no ffmpeg")
	}
	dur := videoDuration(ctx, path)
	// a second in skips the black first frame most clips start with
	at := min(time.Second, dur/3)
	tmp, err := os.CreateTemp("", "beepertui-poster-*.png")
	if err != nil {
		return "", 0, err
	}
	tmp.Close()
	cmd := exec.CommandContext(ctx, ff, "-v", "error", "-y", "-ss", fmt.Sprintf("%.2f", at.Seconds()), "-i", path,
		"-frames:v", "1", "-vf", "scale='min(800,iw)':-2", tmp.Name())
	if err := cmd.Run(); err != nil {
		os.Remove(tmp.Name())
		return "", 0, fmt.Errorf("ffmpeg: %w", err)
	}
	return tmp.Name(), dur, nil
}

func videoDuration(ctx context.Context, path string) time.Duration {
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0
	}
	out, err := exec.CommandContext(ctx, probe, "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0
	}
	s, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return time.Duration(s * float64(time.Second))
}

// videoLabel is the line under (or instead of) the poster.
func (m *Model) videoLabel(a beeper.Attachment, src string) string {
	l := "▶ "
	if t := m.thumbs[src]; t != nil && t.dur > 0 {
		d := t.dur.Round(time.Second)
		l += fmt.Sprintf("%d:%02d ", int(d.Minutes()), int(d.Seconds())%60)
	}
	l += firstNonEmpty(a.FileName, "video") + "  (" + m.km.first("select") + " plays)"
	return l
}

type videoDoneMsg struct{ err error }

// playVideo downloads the video and runs mpv on it, or opens it in the
// default app when mpv isn't installed.
func (m *Model) playVideo(a beeper.Attachment) tea.Cmd {
	src := a.SrcURL
	mode := m.imageMode
	return m.run("", func(ctx context.Context) (func(*Model) tea.Cmd, error) {
		path, err := m.localPath(ctx, src)
		if err != nil {
			return nil, err
		}
		mpv, err := exec.LookPath("mpv")
		if err != nil {
			return func(m *Model) tea.Cmd {
				m.setStatus("no mpv, opened in the default app")
				return nil
			}, openURL("file://" + path)
		}
		vo := "tct" // coloured text art, works in any terminal
		// tuios and tmux sit between us and the terminal and don't pass mpv's kitty images through
		if mode == imagesKitty && os.Getenv("TMUX") == "" && os.Getenv("TUIOS_WINDOW_ID") == "" {
			vo = "kitty"
		}
		c := exec.Command(mpv, "--vo="+vo, "--really-quiet", "--keep-open=no", "--title=beepertui", path)
		return func(m *Model) tea.Cmd {
			return tea.ExecProcess(c, func(err error) tea.Msg { return videoDoneMsg{err} })
		}, nil
	})
}

// afterVideo puts the screen back: mpv may have cleared it and dropped our
// kitty images.
func (m *Model) afterVideo(err error) tea.Cmd {
	if err != nil {
		m.setErr("mpv: " + err.Error())
	}
	return tea.Batch(m.relayout(), m.resendImages())
}
