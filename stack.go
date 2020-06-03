package main

import (
	"runtime"
	"strings"
)

// PanicStack gets the stack as a string, minus the panic frames
func PanicStack() string {
	buf := make([]byte, 2048)
	runtime.Stack(buf, false)
	str := string(buf)
	lines := strings.Split(str, "\n")
	lines = append(lines[:1], lines[7:]...)
	return strings.Join(lines, "\n")
}

// Stack gets the stack as a string
func Stack() string {
	buf := make([]byte, 2048)
	runtime.Stack(buf, false)
	str := string(buf)
	lines := strings.Split(str, "\n")
	lines = append(lines[:1], lines[3:]...)
	return strings.Join(lines, "\n")
}

// ShortStack returns the first caller of the stack
func ShortStack() string {
	buf := make([]byte, 256)
	runtime.Stack(buf, false)
	str := string(buf)
	lines := strings.Split(str, "\n")
	lines = append(lines[:1], lines[3:]...)
	return strings.Join(lines[1:3], "\n")
}
