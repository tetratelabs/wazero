package main

import (
	"io"
	"os"
)

func main() {
	fileIn, err := os.Open("input.txt")
	if err != nil {
		panic(err)
	}
	defer fileIn.Close()

	fileOut, err := os.Create("output.txt")
	if err != nil {
		panic(err)
	}
	defer fileOut.Close()

	_, err = io.Copy(fileOut, fileIn)
	if err != nil {
		panic(err)
	}
}
