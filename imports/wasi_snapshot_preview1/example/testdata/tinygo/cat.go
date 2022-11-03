package main

import (
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

		_, err = file.Stat()
		if err != nil {
			os.Exit(1)
		}

		bytes, err := io.ReadAll(file)
		if err != nil {
			os.Exit(1)
		}

		// Use write to avoid needing to worry about Windows newlines.
		os.Stdout.Write(bytes)
	}
}
