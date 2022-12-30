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

	// Ensure stat works, particularly mode.
	for _, path := range []string{"sub", "/test.txt", "test.txt"} {
		if stat, err := os.Stat(path); err != nil {
			log.Panicln(err)
		} else {
			fmt.Println(path, "mode", stat.Mode())
		}
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
	fmt.Println("empty:" + string(b))
}
