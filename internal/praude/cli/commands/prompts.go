package commands

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func promptLine(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	if prompt != "" {
		fmt.Fprint(out, prompt)
	}
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptYesNo(reader *bufio.Reader, out io.Writer, prompt string) (bool, error) {
	for {
		line, err := promptLine(reader, out, prompt)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(out, "Please answer y or n.")
		}
	}
}
