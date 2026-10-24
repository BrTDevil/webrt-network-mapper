package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// readPassword citește parola fără s-o afișeze (dacă stdin e terminal).
func readPassword() string {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err == nil {
			return string(b)
		}
	}
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

func jsonEncoder(w io.Writer) *json.Encoder {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc
}
