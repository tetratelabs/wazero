package stdio

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

func Main() {
	bufLen, err := strconv.Atoi(os.Getenv("BUF_LEN"))
	if err != nil {
		panic(err)
	}
	buf := make([]byte, bufLen)

	n, err := io.ReadFull(os.Stdin, buf)
	if err != nil {
		panic(err)
	}

	printToFile("stdout", os.Stdout, n)
	printToFile("stderr", os.Stderr, n)
}

func printToFile(name string, file *os.File, size int) {
	message := fmt.Sprint(name, " ", size)
	n, err := fmt.Fprintln(file, message)
	if err != nil {
		println(err.Error())
		panic(name)
	}
	if n != len(message)+1 /* \n */ {
		println(n, "!=", len(message))
		panic(name)
	}
}
