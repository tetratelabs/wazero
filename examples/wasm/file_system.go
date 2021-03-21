//go:generate tinygo build -opt=s -o file_system.wasm -target wasi file_system.go
package main

import (
	"io"
	"os"
)

func main() {
	fileIn, err := os.Open("input.txt")
	if err != nil {
		println(err)
		return
	}
	defer fileIn.Close()

	fileOut, err := os.Create("output.txt")
	if err != nil {
		println(err)
		return
	}
	defer fileOut.Close()

	_, err = io.Copy(fileOut, fileIn)
	if err != nil {
		println(err)
		return
	}
}
