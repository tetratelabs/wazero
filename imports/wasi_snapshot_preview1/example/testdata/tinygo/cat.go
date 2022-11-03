package main

import (
	"fmt"
	"io"
	"os"
)

// main runs cat: concatenate and print files.
//
// Note: main becomes WASI's "_start" function.
func main() {
	// Start at arg[1] because args[0] is the program name.
	for i := 1; i < len(os.Args); i++ {
		file, err := os.Open(os.Args[i])
		if err != nil {
			os.Exit(1)
		}

		defer file.Close()

		stat, err := file.Stat()
		if err != nil {
			os.Exit(1)
		}

		os.Stdout.Write([]byte(fmt.Sprintf("Size: %d\n", stat.Size())))
		os.Stdout.Write([]byte(fmt.Sprintf("Mode: %d\n", stat.Mode())))

		bytes, err := io.ReadAll(file)
		if err != nil {
			os.Exit(1)
		}

		// Use write to avoid needing to worry about Windows newlines.
		os.Stdout.Write([]byte(fmt.Sprintf("Content: %s", string(bytes))))
	}
}
