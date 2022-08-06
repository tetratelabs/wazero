package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const gitHubRepoAPI = "https://api.github.com/repos/tetratelabs/wazero"

type gitHubRepo struct {
	Stars int `json:"stargazers_count"`
}

func main() {
	req, err := http.NewRequest("GET", gitHubRepoAPI, nil)
	if err != nil {
		panic(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(resp.Body)
		panic("GitHub lookup failed: " + string(b))
	}

	var repo gitHubRepo
	json.NewDecoder(resp.Body).Decode(&repo)
	fmt.Println("wazero has", repo.Stars, "stars. Does that include you?")
}
