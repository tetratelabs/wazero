package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

func main() {
	var sub bool
	flag.BoolVar(&sub, "sub", false, "whether to subtract arguments instead of add")

	flag.Parse()

	if flag.NArg() < 2 {
		fmt.Println("bad arguments")
		os.Exit(1)
	}

	a, err := strconv.Atoi(flag.Arg(0))
	if err != nil {
		fmt.Println("bad arguments")
		os.Exit(1)
	}

	b, err := strconv.Atoi(flag.Arg(1))
	if err != nil {
		fmt.Println("bad arguments")
		os.Exit(1)
	}

	var res int
	if sub {
		res = a - b
	} else {
		res = a + b
	}

	fmt.Printf("result: %d\n", res)
}
