package main

import (
	"fmt"
	"go-kilo/internal"
	"os"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// ----------------------------------------------------------------------------
// data
// ----------------------------------------------------------------------------
type editorConfig struct {
	cursorX    int
	cursorY    int
	screenRows int
	screenCols int
	fd         int
	state      *term.State
}

var E editorConfig

// ----------------------------------------------------------------------------
// terminal
// ----------------------------------------------------------------------------
func die(err error) {
	// @BUG: if called, terminal doesn't go back to cooked mode
	os.Stdout.WriteString("\x1b[2J")
	os.Stdout.WriteString("\x1b[H")
	fmt.Printf("Encountered error: %s\n", err)
	os.Exit(1)
}

func ctrlKey(c byte) byte {
	return c & 0x1f
}

func isCtrl(c byte) bool {
	return c < 32 || c == 127
}

func editorReadKey(buf []byte) (bool, error) {
	numBytes, err := unix.Read(E.fd, buf)
	return numBytes != 0, err
}

func getCursorPosition() error {
	if n, err := os.Stdout.WriteString("\x1b[6n"); n != 4 {
		return err
	}

	os.Stdout.WriteString("\r\n")
	var buf [1]byte
	for {
		n, err := os.Stdin.Read(buf[:])
		if err != nil {
			return err
		}

		if n != 1 {
			break
		}

		if isCtrl(buf[0]) {
			fmt.Printf("%d\r\n", buf[0])
		} else {
			fmt.Printf("%d (%c)\r\n", buf[0], buf[0])
		}
	}

	editorReadKey(buf[:])

	return nil
}

func editorWindowSize() error {
	ws, err := unix.IoctlGetWinsize(E.fd, unix.TIOCGWINSZ)
	if err != nil {
		if n, err := os.Stdout.WriteString("\x1b[999C\x1b[999B"); n != 12 {
			fmt.Printf("Error: %s\n", err)
			return fmt.Errorf("getCursorPosition: write failed")
		}

		return getCursorPosition()
	}

	if ws.Col == 0 || ws.Row == 0 {
		return fmt.Errorf("Window size too small.\n")
	}

	E.screenRows = int(ws.Row)
	E.screenCols = int(ws.Col)

	return nil
}

// ----------------------------------------------------------------------------
// output
// ----------------------------------------------------------------------------
func editorDrawRows() {
	for range E.screenRows {
		os.Stdout.WriteString("~\r\n")
	}
}

func editorRefreshScreen() {
	os.Stdout.WriteString("\x1b[2J")
	os.Stdout.WriteString("\x1b[H")
	editorDrawRows()
	os.Stdout.WriteString("\x1b[H")
}

// ----------------------------------------------------------------------------
// input
// ----------------------------------------------------------------------------
func editorProcessKey(buf []byte) error {
	bytesToRead, err := editorReadKey(buf)
	if err != nil {
		return err
	}

	if bytesToRead {
		switch buf[0] {
		case ctrlKey('q'):
			os.Stdout.WriteString("\x1b[2J")
			os.Stdout.WriteString("\x1b[H")

			return fmt.Errorf("User quit.\n")
		default:
			fmt.Printf("%c (%d)\r\n", buf[0], buf[0])
		}
	}

	return nil
}

// ----------------------------------------------------------------------------
// init
// ----------------------------------------------------------------------------
func main() {
	E.fd = int(os.Stdin.Fd())

	var err error
	E.state, err = term.MakeRaw(E.fd)
	if err != nil {
		fmt.Printf("Unable to interact with terminal: %s\n", err)
		return
	}
	defer term.Restore(E.fd, E.state)

	t, err := unix.IoctlGetTermios(E.fd, internal.GetTermios)
	if err != nil {
		fmt.Printf("unix.IoctlGetTermios error: %s\n", err)
		return
	}

	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 1

	if err := unix.IoctlSetTermios(E.fd, internal.SetTermios, t); err != nil {
		fmt.Printf("unix.IoctlSetTermios error: %s\n", err)
		return
	}

	err = editorWindowSize()
	if err != nil {
		fmt.Printf("Failed to find window: %s\n", err)
		return
	}

	if E.screenCols == 0 || E.screenRows == 0 {
		fmt.Printf("Window too small")
		return
	}

	buf := make([]byte, 1)
	for {
		editorRefreshScreen()
		err := editorProcessKey(buf)
		if err != nil {
			fmt.Printf("%s\n", err)
			break
		}
	}
}
