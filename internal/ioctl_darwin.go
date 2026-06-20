//go:build darwin

package internal

import "golang.org/x/sys/unix"

const (
	GetTermios = unix.TIOCGETA
	SetTermios = unix.TIOCSETA
)
