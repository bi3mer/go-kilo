package main

import (
	"bufio"
	"fmt"
	"os"

	"golang.org/x/term"
)

func main() {
	// ---------------------------------------------------------------------------
	// Enable raw mode (i.e. character input doesn't echo) using Go's library
	// term rather than setting the flags ourselves.
	oldFD, err := term.MakeRaw(int(os.Stdin.Fd())) // fd -> file descriptor
	if err != nil {
		fmt.Printf("Unable to interact with terminal: %s\n", err)
		return
	}

	// Diable raw mode after the program quits. Will still be called on error.
	defer term.Restore(int(os.Stdin.Fd()), oldFD)

	// ---------------------------------------------------------------------------
	// Reader user input, byte-by-byte
	reader := bufio.NewReader(os.Stdin)

	for {
		userInput, err := reader.ReadByte()

		if err != nil {
			fmt.Printf("Encountered error: %s\n", err)
			return
		}

		if userInput == 'q' {
			fmt.Println("Bye!")
			return
		}

		fmt.Printf("Char: %c\r\n", userInput)
	}
}
