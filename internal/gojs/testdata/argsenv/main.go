package main

import (
	_ "flag" // to ensure flags parse
	"fmt"
	"os"
)

func main() {
	fmt.Println()
	for i, a := range os.Args[1:] {
		fmt.Println("args", i, "=", a)
	}
	for i, e := range os.Environ() {
		fmt.Println("environ", i, "=", e)
	}
}
