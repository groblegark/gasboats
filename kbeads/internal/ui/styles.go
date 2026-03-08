package ui

import "fmt"

// ANSI256 color codes matching the Ayu palette.
const (
	colorAccent = 74  // blue
	colorCmd    = 250 // light gray
	colorMuted  = 245 // medium gray
)

var noColor bool

// RenderAccent returns s in the accent (blue) color.
func RenderAccent(s string) string {
	if noColor {
		return s
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", colorAccent, s)
}

// RenderMuted returns s in the muted (gray) color.
func RenderMuted(s string) string {
	if noColor {
		return s
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", colorMuted, s)
}

// RenderCommand returns s styled as a command name (light gray).
func RenderCommand(s string) string {
	if noColor {
		return s
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", colorCmd, s)
}

// ForceNoColor disables color output globally.
func ForceNoColor() {
	noColor = true
}
