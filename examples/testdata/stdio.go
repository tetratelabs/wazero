package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		line := s.Text()
		if _, err := fmt.Printf("Hello, %s!\n", strings.TrimSpace(line)); err != nil {
			os.Exit(1)
		}

		if _, err := fmt.Fprintln(os.Stderr, "Error Message"); err != nil {
			os.Exit(1)
		}
	}
}
