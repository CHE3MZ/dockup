// Package picker is a minimal numbered stdin prompt (no TUI deps).
package picker

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Pick shows options and returns the chosen entry. Empty options errors.
func Pick(prompt string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("nothing to pick from")
	}
	fmt.Println(prompt)
	for i, o := range options {
		fmt.Printf("  [%d] %s\n", i+1, o)
	}
	fmt.Print("Select number: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(options) {
		return "", fmt.Errorf("invalid selection")
	}
	return options[n-1], nil
}

// Confirm prints an ARE YOU SURE (y/N) prompt, true only on y/Y.
func Confirm(prompt string) bool {
	fmt.Printf("%s (y/N): ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}
