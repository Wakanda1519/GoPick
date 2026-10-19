//go:build !windows

package main

// enableUTF8Console на не-Windows системах ничего не делает: консоль там
// работает в UTF-8 по умолчанию.
func enableUTF8Console() {}

// enableANSIConsole на не-Windows системах ничего не делает: ANSI-цвета там
// поддерживаются изначально.
func enableANSIConsole() {}
