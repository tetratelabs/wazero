//go:generate tinygo build -opt=s -o stdio.wasm -target wasi stdio.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		println(err)
		return
	}

	if _, err = fmt.Printf("Hello, %s!\n", strings.TrimSpace(line)); err != nil {
		println(err)
		return
	}

	if _, err = fmt.Fprintln(os.Stderr, "Error Message"); err != nil {
		println(err)
		return
	}
}
