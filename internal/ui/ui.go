package ui

import (
	"fmt"
	"os"
	"strings"
)

const barWidth = 24

var (
	terminal bool
	color    bool
)

func Init(isTerminal bool) {
	terminal = isTerminal
	color = isTerminal && os.Getenv("NO_COLOR") == ""
}

func Terminal() bool { return terminal }

func paint(code, s string) string {
	if !color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func green(s string) string { return paint("32", s) }
func red(s string) string   { return paint("31", s) }
func bold(s string) string  { return paint("1", s) }
func dim(s string) string   { return paint("2", s) }

func Green(s string) string { return green(s) }
func Red(s string) string   { return red(s) }
func Bold(s string) string  { return bold(s) }
func Dim(s string) string   { return dim(s) }

func Step(n, total int, title string) {
	fmt.Printf("\n%s\n", bold(fmt.Sprintf("[%d/%d] %s", n, total, title)))
}

func OK(format string, args ...any) {
	fmt.Printf(" %s  %s\n", green("OK"), fmt.Sprintf(format, args...))
}

func Info(format string, args ...any) {
	fmt.Printf("     %s\n", fmt.Sprintf(format, args...))
}

func Warn(format string, args ...any) {
	fmt.Printf(" %s  %s\n", red("!!"), fmt.Sprintf(format, args...))
}

func Progress(done, total, succeeded, failed int) {
	if !terminal || total == 0 {
		return
	}

	ratio := float64(done) / float64(total)
	filled := int(ratio*barWidth + 0.5)
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled)

	line := fmt.Sprintf("%s %d/%d (%.1f%%)  успешно: %d  ошибок: %d",
		bar, done, total, ratio*100, succeeded, failed)
	if failed > 0 {
		line = strings.Replace(line, fmt.Sprintf("ошибок: %d", failed), red(fmt.Sprintf("ошибок: %d", failed)), 1)
	}
	fmt.Printf("\r %s", line)
}

func ProgressDone() {
	if terminal {
		fmt.Println()
	}
}
