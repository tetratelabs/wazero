package stdio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

func Main() {
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	if _, err = fmt.Fprintln(os.Stdin, " "); errors.Unwrap(err) != syscall.EBADF {
		panic(fmt.Sprint(err.Error(), "!=", syscall.EBADF))
	}
	printToFile("stdout", os.Stdout, len(b))
	printToFile("stderr", os.Stderr, len(b))
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
