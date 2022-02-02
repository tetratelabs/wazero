package main

import (
	"fmt"
	"os"
)

func main() {
}

//export PrintArgs
func PrintArgs() {
	fmt.Printf("os.Args: %v", os.Args)
}
