// Package process is an integration test of system calls mapped to the
// JavaScript object "process". e.g. `go.syscall/js.valueCall(process.chdir...`
package process

import (
	"fmt"
	"log"
	"os"
	"syscall"
)

func Main() {
	fmt.Printf("syscall.Getpid()=%d\n", syscall.Getpid())
	fmt.Printf("syscall.Getppid()=%d\n", syscall.Getppid())
	fmt.Printf("syscall.Getuid()=%d\n", syscall.Getuid())
	fmt.Printf("syscall.Getgid()=%d\n", syscall.Getgid())
	fmt.Printf("syscall.Geteuid()=%d\n", syscall.Geteuid())
	fmt.Printf("syscall.Umask(0077)=%O\n", syscall.Umask(0o077))
	if g, err := syscall.Getgroups(); err != nil {
		log.Panicln(err)
	} else {
		fmt.Printf("syscall.Getgroups()=%v\n", g)
	}

	pid := syscall.Getpid()
	if p, err := os.FindProcess(pid); err != nil {
		log.Panicln(err)
	} else {
		fmt.Printf("os.FindProcess(%d).Pid=%d\n", pid, p.Pid)
	}

	if wd, err := syscall.Getwd(); err != nil {
		log.Panicln(err)
	} else if wd != "/" {
		log.Panicln("not root")
	}
	fmt.Println("wd ok")

	dirs := []struct {
		path, wd string
	}{
		{"dir", "/dir"},
		{".", "/dir"},
		{"..", "/"},
		{".", "/"},
		{"..", "/"},
	}

	for _, dir := range dirs {
		if err := syscall.Chdir(dir.path); err != nil {
			log.Panicln(dir.path, err)
		} else if wd, err := syscall.Getwd(); err != nil {
			log.Panicln(dir.path, err)
		} else if wd != dir.wd {
			log.Panicf("cd %s: expected wd=%s, but have %s", dir.path, dir.wd, wd)
		}
	}

	if err := syscall.Chdir("/animals.txt"); err == nil {
		log.Panicln("shouldn't be able to chdir to file")
	} else {
		fmt.Println(err) // should be the textual message of the errno.
	}
}
