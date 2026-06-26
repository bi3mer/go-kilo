package main

import (
	"fmt"
	"go-kilo/internal"
	"os"
	"strings"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const kiloVersion = "0.0.0"

const (
	arrowLeft = iota + 1000
	arrowRight
	arrowUp
	arrowDown
	homeKey
	endKey
	pageUp
	pageDown
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
func ctrlKey(c byte) byte {
	return c & 0x1f
}

func editorReadKey() (int, error) {
	var buf [1]byte
	for {
		numBytes, err := unix.Read(E.fd, buf[:])
		if err != nil {
			return -1, err
		}

		if numBytes > 0 {
			break
		}
	}

	if buf[0] == '\x1b' {
		var seq [3]byte

		numBytes, err := unix.Read(E.fd, seq[0:1])
		if err != nil {
			return -1, err
		}
		if numBytes != 1 {
			return '\x1b', nil
		}

		numBytes, err = unix.Read(E.fd, seq[1:2])
		if err != nil {
			return -1, err
		}
		if numBytes != 1 {
			return '\x1b', nil
		}

		if seq[0] == '[' {
			if seq[1] >= '0' && seq[1] <= '9' {
				numBytes, err = unix.Read(E.fd, seq[2:3])
				if err != nil {
					return -1, err
				}
				if numBytes != 1 {
					return '\x1b', nil
				}

				if seq[2] == '~' {
					switch seq[1] {
					case '1':
						return homeKey, nil
					case '4':
						return endKey, nil
					case '5':
						return pageUp, nil
					case '6':
						return pageDown, nil
					case '7':
						return homeKey, nil
					case '8':
						return endKey, nil
					}
				}
			} else {
				switch seq[1] {
				case 'A':
					return arrowUp, nil
				case 'B':
					return arrowDown, nil
				case 'C':
					return arrowRight, nil
				case 'D':
					return arrowLeft, nil
				case 'H':
					return homeKey, nil
				case 'F':
					return endKey, nil
				}
			}
		} else if seq[0] == 'O' {
			switch seq[1] {
			case 'H':
				return homeKey, nil
			case 'F':
				return endKey, nil
			}
		}

		return '\x1b', nil
	}

	return int(buf[0]), nil
}

func getCursorPosition() error {
	n, err := os.Stdout.WriteString("\x1b[6n")
	if err != nil {
		return err
	}

	if n != 4 {
		return fmt.Errorf("getCursorPosition: write failed")
	}

	var buf [32]byte
	var i int
	for i = 0; i < len(buf)-1; i++ {
		n, err := unix.Read(E.fd, buf[i:i+1])
		if err != nil {
			return err
		}

		if n != 1 {
			break
		}

		if buf[i] == 'R' {
			break
		}
	}

	buf[i] = 0

	if buf[0] != '\x1b' || buf[1] != '[' {
		return fmt.Errorf("getCursorPosition: unexpected response")
	}

	n, err = fmt.Sscanf(string(buf[2:i]), "%d;%d", &E.screenRows, &E.screenCols)
	if err != nil {
		return err
	}

	if n != 2 {
		return fmt.Errorf("getCursorPosition: expected 2 values, got %d", n)
	}

	return nil
}

func editorWindowSize() error {
	ws, err := unix.IoctlGetWinsize(E.fd, unix.TIOCGWINSZ)
	if err != nil {
		if n, err := os.Stdout.WriteString("\x1b[999C\x1b[999B"); err != nil || n != 12 {
			return fmt.Errorf("getCursorPosition: write failed: %w", err)
		}

		return getCursorPosition()
	}

	if ws.Col == 0 || ws.Row == 0 {
		return fmt.Errorf("window size too small")
	}

	E.screenRows = int(ws.Row)
	E.screenCols = int(ws.Col)

	return nil
}

// ----------------------------------------------------------------------------
// output
// ----------------------------------------------------------------------------
func editorDrawRows(ab *strings.Builder) {
	for y := range E.screenRows {
		if y == E.screenRows/3 {
			welcomeMessage := fmt.Sprintf("Kilo Editor -- v%s", kiloVersion)

			if len(welcomeMessage) > E.screenCols {
				welcomeMessage = welcomeMessage[:E.screenCols]
			}

			padding := (E.screenCols - len(welcomeMessage)) / 2
			if padding > 0 {
				ab.WriteByte('~')
				padding--
			}

			for padding > 0 {
				ab.WriteByte(' ')
				padding--
			}

			ab.WriteString(welcomeMessage)

		} else {
			ab.WriteString("~")
		}

		ab.WriteString("\x1b[K") // erase terminal row

		if y < E.screenRows-1 {
			ab.WriteString("\r\n")
		}
	}
}

func editorRefreshScreen() {
	var ab strings.Builder

	ab.WriteString("\x1b[?25l") // hide cursor
	ab.WriteString("\x1b[H")    // cursor to top-left

	editorDrawRows(&ab)

	ab.WriteString(fmt.Sprintf("\x1b[%d;%dH", E.cursorY+1, E.cursorX+1)) // set cursor position

	ab.WriteString("\x1b[?25h") // show cursor

	_, _ = os.Stdout.WriteString(ab.String())
}

// ----------------------------------------------------------------------------
// input
// ----------------------------------------------------------------------------
func editorMoveCursor(key int) {
	switch key {
	case arrowLeft:
		E.cursorX = max(0, E.cursorX-1)
	case arrowRight:
		E.cursorX = min(E.cursorX+1, E.screenCols-1)
	case arrowUp:
		E.cursorY = max(0, E.cursorY-1)
	case arrowDown:
		E.cursorY = min(E.cursorY+1, E.screenRows-1)
	}
}

func editorProcessKey() error {
	key, err := editorReadKey()
	if err != nil {
		return err
	}

	switch key {
	case int(ctrlKey('q')):
		_, _ = os.Stdout.WriteString("\x1b[2J")
		_, _ = os.Stdout.WriteString("\x1b[H")

		return fmt.Errorf("user quit")

	case homeKey:
		E.cursorX = 0

	case endKey:
		E.cursorX = E.screenCols - 1

	case pageUp, pageDown:
		var press int
		if key == pageUp {
			press = arrowUp
		} else {
			press = arrowDown
		}

		for range E.screenRows {
			editorMoveCursor(press)
		}

	case arrowLeft, arrowRight, arrowDown, arrowUp:
		editorMoveCursor(key)
	}

	return nil
}

// ----------------------------------------------------------------------------
// init
// ----------------------------------------------------------------------------
func main() {
	E.cursorX = 0
	E.cursorY = 0
	E.fd = int(os.Stdin.Fd())

	var err error
	E.state, err = term.MakeRaw(E.fd)
	if err != nil {
		fmt.Fprintf(os.Stderr,"Unable to interact with terminal: %s\n", err)
		return
	}
	defer term.Restore(E.fd, E.state)

	t, err := unix.IoctlGetTermios(E.fd, internal.GetTermios)
	if err != nil {
		fmt.Fprintf(os.Stderr,"unix.IoctlGetTermios error: %s\n", err)
		return
	}

	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 1

	if err := unix.IoctlSetTermios(E.fd, internal.SetTermios, t); err != nil {
		fmt.Fprintf(os.Stderr,"unix.IoctlSetTermios error: %s\n", err)
		return
	}

	err = editorWindowSize()
	if err != nil {
		fmt.Fprintf(os.Stderr,"Failed to find window: %s\n", err)
		return
	}

	if E.screenCols == 0 || E.screenRows == 0 {
		fmt.Fprintf(os.Stderr,"Window too small")
		return
	}

	for {
		editorRefreshScreen()
		err := editorProcessKey()
		if err != nil {
			fmt.Fprintf(os.Stderr,"%s\n", err)
			break
		}
	}
}
