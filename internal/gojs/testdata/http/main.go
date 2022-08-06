package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	res, err := http.Get(os.Args[0] + "/error")
	if err == nil {
		log.Panicln(err)
	}
	fmt.Println(err)

	res, err = http.Post(os.Args[0], "text/plain", io.NopCloser(strings.NewReader("ice cream")))
	if err != nil {
		log.Panicln(err)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Panicln(err)
	}
	res.Body.Close()

	fmt.Println(res.Header.Get("Custom"))
	fmt.Println(string(body))
}
