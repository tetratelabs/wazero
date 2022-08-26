package http

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func Main() {
	url := os.Getenv("BASE_URL")
	res, err := http.Get(url + "/error")
	if err == nil {
		log.Panicln(err)
	}
	fmt.Println(err)

	res, err = http.Post(url, "text/plain", io.NopCloser(strings.NewReader("ice cream")))
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
