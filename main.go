package main

import (
	"fmt"
	"go-kilo/internal"
	"os"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// @BUG: if called, terminal doesn't go back to cooked mode
func die(err error) {
	fmt.Printf("Encountered error: %s\n", err)
	os.Exit(1)
}

func ctrlKey(c byte) byte {
	return c & 0x1f
}

func editorReadKey(fd int, buf []byte) bool{
	// get user input
	numBytes, err := unix.Read(fd, buf)
	if err != nil {
		die(err)
	}

	// If numBytes == 0, then time since user input exceeded unix.VTIME max.
	// So we don't process input and end the loop.
	return numBytes != 0
}

func editorProcessKey(fd int, buf []byte) bool {
	running := true
	if (editorReadKey(fd, buf)) {
		switch buf[0] {
			case ctrlKey('q'):
				fmt.Printf("Bye!\r\n")
				running = false
			default:
				fmt.Printf("%c (%d)\r\n", buf[0], buf[0])
		}
	}

	return running
}

func main() {
	// ---------------------------------------------------------------------------
	// Enable raw mode (i.e. character input doesn't echo) using Go's library
	// term rather than setting the flags ourselves.
	fd := int(os.Stdin.Fd()) // file descriptor
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		fmt.Printf("Unable to interact with terminal: %s\n", err)
		return
	}

	// Diable raw mode after the program quits. Will still be called on error.
	defer term.Restore(fd, oldState)

	// ---------------------------------------------------------------------------
	// grab the current termios so we can update on failed reads
	t, err := unix.IoctlGetTermios(fd, internal.GetTermios)
	if err != nil {
		fmt.Printf("unix.IoctlGetTermios error: %s\n", err)
		return
	}

	t.Cc[unix.VMIN] = 0  // min bytes to read from user
	t.Cc[unix.VTIME] = 1 // max time before reading from user returns

	if err := unix.IoctlSetTermios(fd, internal.SetTermios, t); err != nil {
		fmt.Printf("unix.IoctlSetTermios error: %s\n", err)
		return
	}

	// ---------------------------------------------------------------------------
	// Reader user input, byte-by-byte
	buf := make([]byte, 1)
	running := true
	for running {
		running = editorProcessKey(fd, buf)
	}
}
