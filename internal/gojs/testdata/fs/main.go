package fs

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"testing/fstest"
)

func Main() {
	testFS()
	testAdHoc()
}

func testFS() {
	if err := fstest.TestFS(os.DirFS("sub"), "test.txt"); err != nil {
		log.Panicln("TestFS err:", err)
	}
	fmt.Println("TestFS ok")
}

func testAdHoc() {
	if wd, err := syscall.Getwd(); err != nil {
		log.Panicln(err)
	} else if wd != "/" {
		log.Panicln("not root")
	}
	fmt.Println("wd ok")

	if err := syscall.Chdir("/test.txt"); err == nil {
		log.Panicln("shouldn't be able to chdir to file")
	} else {
		fmt.Println(err) // should be the textual message of the errno.
	}

	for _, path := range []string{"/test.txt", "test.txt"} {
		s, err := os.Stat(path)
		if err != nil {
			log.Panicln(err)
		}
		if s.IsDir() {
			log.Panicln(path, "is dir")
		}
		fmt.Println(path, "ok")
	}

	b, err := os.ReadFile("/test.txt")
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println("contents:", string(b))

	b, err = os.ReadFile("/empty.txt")
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println("empty:", string(b))
}
