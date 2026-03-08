package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/groblegark/kbeads/internal/ui"
	"github.com/spf13/cobra"
)

// Patterns used to colorize Cobra's default help output.
var (
	// Section headers: unindented line ending with ":" (e.g. "Beads:", "Flags:").
	// Excludes "Usage:" which we leave unstyled.
	reGroupHeader = regexp.MustCompile(`(?m)^([A-Z][^\n]*:)\s*$`)

	// Command names: two-space indent, then a word, then two-or-more spaces
	// before the description.
	reCommand = regexp.MustCompile(`(?m)^(  )(\S+)(  )`)

	// Flag type annotations: e.g. "--server string", "--limit int32".
	reFlagType = regexp.MustCompile(`(--?\S+\s+)(string|int|int32|duration|stringSlice|stringArray)`)

	// Default values in square brackets that look like defaults, e.g.
	// (default "foo"). Only match brackets containing "default" or
	// starting with a quote to avoid matching [command], [flags], etc.
	reDefault = regexp.MustCompile(`\(default "[^"]*"\)`)
)

// colorizedHelpFunc returns a Cobra help function that post-processes the
// default help text with ANSI colors when the terminal supports it.
func colorizedHelpFunc() func(*cobra.Command, []string) {
	defaultHelp := func(cmd *cobra.Command, args []string) {
		cmd.SetOut(cmd.OutOrStdout())
		_ = cmd.Usage()
	}

	return func(cmd *cobra.Command, args []string) {
		if !ui.ShouldUseColor() {
			defaultHelp(cmd, args)
			return
		}

		// Save the original writer before redirecting to a buffer.
		orig := cmd.OutOrStdout()

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		_ = cmd.Usage()
		cmd.SetOut(orig)

		colorized := colorizeHelpOutput(buf.String())
		fmt.Fprint(orig, colorized)
	}
}

// colorizeHelpOutput applies ANSI styling to Cobra's plain-text help.
func colorizeHelpOutput(s string) string {
	// 1. Color section headers.
	s = reGroupHeader.ReplaceAllStringFunc(s, func(match string) string {
		return ui.RenderAccent(strings.TrimSpace(match))
	})

	// 2. Color command names.
	s = reCommand.ReplaceAllStringFunc(s, func(match string) string {
		parts := reCommand.FindStringSubmatch(match)
		if len(parts) == 4 {
			return parts[1] + ui.RenderCommand(parts[2]) + parts[3]
		}
		return match
	})

	// 3. Color flag type annotations.
	s = reFlagType.ReplaceAllStringFunc(s, func(match string) string {
		parts := reFlagType.FindStringSubmatch(match)
		if len(parts) == 3 {
			return parts[1] + ui.RenderMuted(parts[2])
		}
		return match
	})

	// 4. Color default values.
	s = reDefault.ReplaceAllStringFunc(s, func(match string) string {
		return ui.RenderMuted(match)
	})

	return s
}
