package main

import (
	"io"
	"os"
)

// This example reads the input file paths from args except the last argument,
// and writes the concatenated contents to the last file path.
func main() {
	files := os.Args
	inputFiles := files[:len(files)-1]
	outputFile := files[len(files)-1]

	fileOut, err := os.Create(outputFile)
	if err != nil {
		panic(err)
	}
	defer fileOut.Close()

	for _, input := range inputFiles {
		fileIn, err := os.Open(input)
		if err != nil {
			panic(err)
		}
		defer fileIn.Close()

		_, err = io.Copy(fileOut, fileIn)
		if err != nil {
			panic(err)
		}
	}
}
