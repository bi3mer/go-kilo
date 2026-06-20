//go:build linux

package internal

import "golang.org/x/sys/unix"

const (
	GetTermios = unix.TCGETS
	SetTermios = unix.TCSETS
)
