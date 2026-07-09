package main

import (
	"bufio"
	"fmt"
	"go-kilo/internal"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const (
	kiloVersion = "0.0.0"
	tabLength   = 8
)

const (
	backspace = 127
)

const (
	arrowLeft = iota + 1000
	arrowRight
	arrowUp
	arrowDown
	delKey
	homeKey
	endKey
	pageUp
	pageDown
)

// ----------------------------------------------------------------------------
// data
// ----------------------------------------------------------------------------
type editorConfig struct {
	cursorX       int
	cursorY       int
	rowX          int
	rowOff        int
	colOff        int
	screenRows    int
	screenCols    int
	fd            int
	state         *term.State
	statusMsg     string
	statusMsgTime time.Time
	fileName      string
	rows          []string
	render        []string
}

var E editorConfig

// ----------------------------------------------------------------------------
// row operations
// ----------------------------------------------------------------------------
func editorRowCursorXToRenderX(row string, cx int) int {
	rx := 0

	for j := 0; j < cx; j++ {
		if row[j] == '\t' {
			rx += (tabLength - 1) - (rx % tabLength)
		}

		rx++
	}

	return rx
}

func editorUpdateRow(row string) string {
	var builder strings.Builder

	for _, r := range row {
		if r == '\t' {
			builder.WriteByte(' ')

			for builder.Len()%tabLength != 0 {
				builder.WriteByte(' ')
			}
		} else {
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

func editorRowInsertRune(row string, at int, r rune) string {
	if at < 0 || at > len(row) {
		at = len(row)
	}

	return row[:at] + string(r) + row[at:]
}

// ----------------------------------------------------------------------------
// editor operations
// ----------------------------------------------------------------------------

func editorInsertRune(r rune) {
	if E.cursorY == len(E.rows) {
		E.rows = append(E.rows, string(r))
		E.render = append(E.render, editorUpdateRow(string(r)))
	} else {
		E.rows[E.cursorY] = editorRowInsertRune(E.rows[E.cursorY], E.cursorX, r)
		E.render[E.cursorY] = editorUpdateRow(E.rows[E.cursorY])
	}

	E.cursorX++
}

// ----------------------------------------------------------------------------
// file i/o
// ----------------------------------------------------------------------------
func editorOpen(fileName string) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}

	defer f.Close()
	E.fileName = fileName

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		E.rows = append(E.rows, line)
		E.render = append(E.render, editorUpdateRow(line))
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func editorSave() {
	if len(E.fileName) == 0 {
		return
	}

	f, err := os.OpenFile(E.fileName, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		editorSetStatusMessage("Can't save! I/O error: %s", err)
		return
	}
	defer f.Close()

	data := strings.Join(E.rows, "\n") + "\n"
	if err := f.Truncate(int64(len(data))); err != nil {
		editorSetStatusMessage("Can't save! I/O error: %s", err)
		return
	}

	if _, err := f.Write([]byte(data)); err != nil {
		editorSetStatusMessage("Can't save! I/O error: %s", err)
		return
	}

	editorSetStatusMessage("%d bytes written to disk", len(data))
}

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
					case '3':
						return delKey, nil
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
func editorScroll() {
	if E.cursorY < len(E.rows) {
		E.rowX = editorRowCursorXToRenderX(E.rows[E.cursorY], E.cursorX)
	} else {
		E.rowX = 0
	}

	if E.cursorY < E.rowOff {
		E.rowOff = E.cursorY
	}

	if E.cursorY >= E.rowOff+E.screenRows {
		E.rowOff = E.cursorY - E.screenRows + 1
	}

	if E.rowX < E.colOff {
		E.colOff = E.rowX
	}

	if E.rowX >= E.colOff+E.screenCols {
		E.colOff = E.rowX - E.screenCols + 1
	}
}

func editorDrawRows(ab *strings.Builder) {
	for y := range E.screenRows {
		fileRow := y + E.rowOff
		if fileRow >= len(E.rows) {
			if len(E.rows) == 0 && y == E.screenRows/3 {
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
		} else {
			// prune lines too long for now
			row := E.render[fileRow]

			if E.colOff < len(row) {
				length := min(len(row)-E.colOff, E.screenCols)
				row = row[E.colOff : E.colOff+length]
			} else {
				row = ""
			}

			ab.WriteString(row)
		}

		ab.WriteString("\x1b[K") // erase terminal row
		ab.WriteString("\r\n")
	}
}

func editorDrawStatusBar(ab *strings.Builder) {
	ab.WriteString("\x1b[7m")

	// draw file name
	var length int
	if E.fileName == "" {
		ab.WriteString("[No Name]")
		length = 9
	} else {
		var temp strings.Builder
		temp.WriteString(E.fileName)
		temp.WriteString(" - ")
		temp.WriteString(strconv.Itoa(len(E.rows)))
		temp.WriteString(" lines")

		length = min(E.screenCols, temp.Len())
		ab.WriteString(temp.String()[:length])
	}

	var lineCount strings.Builder
	lineCount.WriteString(strconv.Itoa(E.cursorY + 1))
	lineCount.WriteByte('/')
	lineCount.WriteString(strconv.Itoa(len(E.rows)))

	for i := range E.screenCols - length {
		if i == E.screenCols-length-lineCount.Len() {
			ab.WriteString(lineCount.String())
			break
		} else {
			ab.WriteByte(' ')
		}
	}

	ab.WriteString("\x1b[m")
	ab.WriteString("\r\n")
}

func editorDrawMessageBar(ab *strings.Builder) {
	ab.WriteString("\x1b[K")

	length := min(len(E.statusMsg), E.screenCols)
	if time.Since(E.statusMsgTime).Seconds() < 5.0 {
		ab.WriteString(E.statusMsg[:length])
	}
}

func editorRefreshScreen() {
	editorScroll()

	var ab strings.Builder

	ab.WriteString("\x1b[?25l") // hide cursor
	ab.WriteString("\x1b[H")    // cursor to top-left

	editorDrawRows(&ab)
	editorDrawStatusBar(&ab)
	editorDrawMessageBar(&ab)

	// set cursor position
	ab.WriteString(fmt.Sprintf("\x1b[%d;%dH", (E.cursorY-E.rowOff)+1, (E.rowX-E.colOff)+1))

	ab.WriteString("\x1b[?25h") // show cursor

	_, _ = os.Stdout.WriteString(ab.String())
}

func editorSetStatusMessage(msg string, args ...any) {
	E.statusMsg = fmt.Sprintf(msg, args...)
	E.statusMsgTime = time.Now()
}

// ----------------------------------------------------------------------------
// input
// ----------------------------------------------------------------------------
func editorMoveCursor(key int) {
	switch key {
	case arrowLeft:
		if E.cursorX != 0 {
			E.cursorX--
		} else if E.cursorY > 0 {
			E.cursorY--
			E.cursorX = len(E.rows[E.cursorY])
		}
	case arrowRight:
		if E.cursorY < len(E.rows) {
			if E.cursorX < len(E.rows[E.cursorY]) {
				E.cursorX++
			} else if E.cursorX == len(E.rows[E.cursorY]) {
				E.cursorY++
				E.cursorX = 0
			}
		}
	case arrowUp:
		E.cursorY = max(0, E.cursorY-1)
	case arrowDown:
		if E.cursorY < len(E.rows) {
			E.cursorY++
		}
	}

	rowLen := 0
	if E.cursorY < len(E.rows) {
		rowLen = len(E.rows[E.cursorY])
	}

	if E.cursorX > rowLen {
		E.cursorX = rowLen
	}
}

func editorProcessKey() error {
	key, err := editorReadKey()
	if err != nil {
		return err
	}

	switch key {
	case '\r':
		// todo

	case int(ctrlKey('q')):
		_, _ = os.Stdout.WriteString("\x1b[2J")
		_, _ = os.Stdout.WriteString("\x1b[H")

		return fmt.Errorf("user quit")

	case ('s' & 0x1f):
		editorSetStatusMessage("hi")
		editorSave()

	case homeKey:
		E.cursorX = 0

	case endKey:
		if E.cursorY < len(E.rows) {
			E.cursorX = len(E.rows[E.cursorY])
		}

	case backspace, int(ctrlKey('h')), delKey:
		// todo

	case pageUp, pageDown:
		var press int
		if key == pageUp {
			press = arrowUp

			E.cursorY = E.rowOff
		} else {
			E.cursorY = E.rowOff + E.screenRows - 1
			if E.cursorY > len(E.rows) {
				E.cursorY = len(E.rows)
			}

			press = arrowDown
		}

		for range E.screenRows {
			editorMoveCursor(press)
		}

	case arrowLeft, arrowRight, arrowDown, arrowUp:
		editorMoveCursor(key)

	case int(ctrlKey('l')), '\x1b':
		// todo

	default:
		if key < 256 {
			editorInsertRune(rune(key))
		} else {
			editorSetStatusMessage("Unhandled key: %d", key)
		}
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
		fmt.Fprintf(os.Stderr, "Unable to interact with terminal: %s\n", err)
		return
	}
	defer term.Restore(E.fd, E.state)

	t, err := unix.IoctlGetTermios(E.fd, internal.GetTermios)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unix.IoctlGetTermios error: %s\n", err)
		return
	}

	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 1

	if err := unix.IoctlSetTermios(E.fd, internal.SetTermios, t); err != nil {
		fmt.Fprintf(os.Stderr, "unix.IoctlSetTermios error: %s\n", err)
		return
	}

	err = editorWindowSize()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find window: %s\n", err)
		return
	}

	if E.screenCols == 0 || E.screenRows == 0 {
		fmt.Fprintf(os.Stderr, "Window too small")
		return
	}

	if len(os.Args) >= 2 {
		err = editorOpen(os.Args[1])

		if err != nil {
			fmt.Fprintf(os.Stderr, "%v", err)
			return
		}
	}

	E.screenRows -= 2

	E.statusMsgTime = time.Now() // default time to current time
	editorSetStatusMessage("HELP: Ctrl-S = save | Ctrl-Q = quit")

	for {
		editorRefreshScreen()

		err := editorProcessKey()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			break
		}
	}
}
