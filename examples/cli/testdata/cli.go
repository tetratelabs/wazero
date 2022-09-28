package main

import (
	"flag"
	"os"
	"strconv"
)

func main() {
	var sub bool
	flag.BoolVar(&sub, "sub", false, "whether to subtract arguments instead of add")

	flag.Parse()

	if flag.NArg() < 2 {
		os.Stdout.WriteString("bad arguments\n")
		os.Exit(1)
	}

	a, err := strconv.Atoi(flag.Arg(0))
	if err != nil {
		os.Stdout.WriteString("bad arguments\n")
		os.Exit(1)
	}

	b, err := strconv.Atoi(flag.Arg(1))
	if err != nil {
		os.Stdout.WriteString("bad arguments\n")
		os.Exit(1)
	}

	var res int
	if sub {
		res = a - b
	} else {
		res = a + b
	}

	os.Stdout.WriteString("result: " + strconv.Itoa(res) + "\n")
}
