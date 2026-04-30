package syncer

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	ansiReset = "\x1b[0m"

	ansiBold    = "\x1b[1m"
	ansiCyan    = "\x1b[36m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiRed     = "\x1b[31m"
	ansiMagenta = "\x1b[35m"
	ansiBlue    = "\x1b[34m"
	ansiGray    = "\x1b[90m"
)

type outputStyle struct {
	enabled bool
}

func newOutputStyle(writer io.Writer) outputStyle {
	return outputStyle{enabled: shouldUseColor(writer)}
}

func shouldUseColor(writer io.Writer) bool {
	// https://no-color.org/
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// CLICOLOR=0 disables color, CLICOLOR_FORCE enables it.
	if os.Getenv("CLICOLOR") == "0" {
		return false
	}

	if os.Getenv("CLICOLOR_FORCE") != "" && os.Getenv("CLICOLOR_FORCE") != "0" {
		return true
	}

	fdWriter, ok := writer.(interface{ Fd() uintptr })
	if !ok {
		return false
	}

	fileDescriptor := fdWriter.Fd()

	maxIntValue := ^uintptr(0) >> 1

	if fileDescriptor > maxIntValue {
		return false
	}

	return term.IsTerminal(int(fileDescriptor))
}

func (s outputStyle) wrap(text, ansiCode string) string {
	if !s.enabled || text == "" {
		return text
	}

	return ansiCode + text + ansiReset
}

func (s outputStyle) hostHeader(text string) string  { return s.wrap(text, ansiBold+ansiCyan) }
func (s outputStyle) projectName(text string) string { return s.wrap(text, ansiBold+ansiMagenta) }
func (s outputStyle) key(text string) string         { return s.wrap(text, ansiBlue) }
func (s outputStyle) success(text string) string     { return s.wrap(text, ansiGreen) }
func (s outputStyle) warning(text string) string     { return s.wrap(text, ansiYellow) }
func (s outputStyle) danger(text string) string      { return s.wrap(text, ansiRed) }
func (s outputStyle) muted(text string) string       { return s.wrap(text, ansiGray) }

func (s outputStyle) actionSymbol(action ActionType) string {
	switch action {
	case ActionAdd:
		return s.success(action.Symbol())
	case ActionModify:
		return s.warning(action.Symbol())
	case ActionDelete:
		return s.danger(action.Symbol())
	case ActionUnchanged:
		return s.muted(action.Symbol())
	default:
		return action.Symbol()
	}
}

func (s outputStyle) diffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return s.key(line)
	case strings.HasPrefix(line, "@@"):
		return s.muted(line)
	case strings.HasPrefix(line, "+"):
		return s.success(line)
	case strings.HasPrefix(line, "-"):
		return s.danger(line)
	default:
		return line
	}
}
