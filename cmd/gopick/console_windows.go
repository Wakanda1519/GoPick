//go:build windows

package main

import (
	"os"
	"syscall"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	procSetConsoleCP       = kernel32.NewProc("SetConsoleCP")
	procSetConsoleMode     = kernel32.NewProc("SetConsoleMode")
)

// enableUTF8Console переключает кодовую страницу консоли Windows на UTF-8,
// иначе рамки баннера и русский текст отображаются как мусор.
func enableUTF8Console() {
	const utf8CodePage = 65001
	procSetConsoleOutputCP.Call(uintptr(utf8CodePage))
	procSetConsoleCP.Call(uintptr(utf8CodePage))
}

// enableANSIConsole включает обработку ANSI-последовательностей, чтобы цвета
// этапов и прогресса работали в cmd и PowerShell.
func enableANSIConsole() {
	const enableVirtualTerminalProcessing = 0x0004

	handle := syscall.Handle(os.Stdout.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(handle, &mode); err != nil {
		return
	}
	procSetConsoleMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminalProcessing))
}
