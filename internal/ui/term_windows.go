//go:build windows

package ui

// Windows consoles don't report pixel sizes, assume the usual 1:2 cell.
func cellAspect() float64 { return 2 }
