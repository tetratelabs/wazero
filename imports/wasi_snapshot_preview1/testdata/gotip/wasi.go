package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"syscall"
)

func main() {
	switch os.Args[1] {
	case "sock":
		if err := mainSock(); err != nil {
			panic(err)
		}
	case "http":
		if err := mainHTTP(); err != nil {
			panic(err)
		}
	}
}

// mainSock is an explicit test of a blocking socket.
func mainSock() error {
	// Get a listener from the pre-opened file descriptor.
	// The listener is the first pre-open, with a file-descriptor of 3.
	f := os.NewFile(3, "")
	l, err := net.FileListener(f)
	defer f.Close()
	if err != nil {
		return err
	}
	defer l.Close()

	// Accept a connection
	conn, err := l.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	// Do a blocking read of up to 32 bytes.
	// Note: the test should write: "wazero", so that's all we should read.
	var buf [32]byte
	n, err := conn.Read(buf[:])
	if err != nil {
		return err
	}
	fmt.Println(string(buf[:n]))
	return nil
}

// mainHTTP implicitly tests non-blocking sockets, as they are needed for
// middleware.
func mainHTTP() error {
	// Get the file representing a pre-opened TCP socket.
	// The socket (listener) is the first pre-open, with a file-descriptor of
	// 3 because the host didn't add any pre-opened files.
	listenerFD := 3
	f := os.NewFile(uintptr(listenerFD), "")

	// Wasm runs similarly to GOMAXPROCS=1, so multiple goroutines cannot work
	// in parallel. non-blocking allows the poller to park the go-routine
	// accepting connections while work is done on one.
	if err := syscall.SetNonblock(listenerFD, true); err != nil {
		return err
	}

	// Convert the file representing the pre-opened socket to a listener, so
	// that we can integrate it with HTTP middleware.
	ln, err := net.FileListener(f)
	defer f.Close()
	if err != nil {
		return err
	}
	defer ln.Close()

	// Serve middleware that echos the request body to the response.
	return http.Serve(ln, echo{})
}

type echo struct{}

func (echo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Copy up to 32 bytes from the request to the response, appending a newline.
	// Note: the test should write: "wazero", so that's all we should read.
	var buf [32]byte
	if n, err := r.Body.Read(buf[:]); err != nil && err != io.EOF {
		panic(err)
	} else if n, err = w.Write(append(buf[:n], '\n')); err != nil {
		panic(err)
	}
}
