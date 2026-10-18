//go:build !windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

// cellAspect is cell height / width in pixels. Kitty and Ghostty report the
// pixel size in TIOCGWINSZ; anything else gets the usual 2.
func cellAspect() float64 {
	var ws struct{ rows, cols, xpix, ypix uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.xpix == 0 || ws.ypix == 0 || ws.cols == 0 || ws.rows == 0 {
		return 2
	}
	return (float64(ws.ypix) / float64(ws.rows)) / (float64(ws.xpix) / float64(ws.cols))
}
