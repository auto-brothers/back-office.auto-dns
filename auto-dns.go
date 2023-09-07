package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

var BACK_OFFICE_URL = os.Getenv("BACK_OFFICE_URL")
var BACK_OFFICE_API_KEY = os.Getenv("BACK_OFFICE_API_KEY")
var intervals = []int{1, 1, 1, 2, 2, 2, 2, 2, 5, 5, 5, 10, 10, 20, 30}
var next_call_index = 30

type ServerResponse struct {
	Id   string
	Body string
}

type ServerRequest struct {
	Status string
	Result string
}

func main() {
	if BACK_OFFICE_URL == "" {
		panic("BACK_OFFICE_URL environment not set")
	}

	if BACK_OFFICE_API_KEY == "" {
		panic("BACK_OFFICE_API_KEY environment not set")
	}

	for {
		callServer()

		next_call_index = min(next_call_index+1, 14)
		log.Printf("Next call in %+v", intervals[next_call_index])

		time.Sleep(time.Second * time.Duration(intervals[next_call_index]))
	}
}

func callServer() {
	client := http.Client{}

	log.Printf("Creating new http request")
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/clients/me/commands", BACK_OFFICE_URL), nil)
	if err != nil {
		log.Printf("http.NewRequest: %+v", err)
		return
	}
	req.Header = http.Header{
		"accept":        {"application/json"},
		"Authorization": {fmt.Sprintf("Basic %s", BACK_OFFICE_API_KEY)},
	}

	res, err := client.Do(req)
	if err != nil {
		log.Printf("client.Do: %+v", err)
		return
	}

	log.Printf("Successfully executed http request. Status code was: %+v", res.StatusCode)

	if res.StatusCode == http.StatusNoContent {
		return
	}

	if res.StatusCode != http.StatusOK {
		log.Printf("Unexpected status code. Response: %+v", res)
		return
	}

	log.Printf("Parsing response")

	decoder := json.NewDecoder(res.Body)
	var serverResponse ServerResponse
	err = decoder.Decode(&serverResponse)
	if err != nil {
		log.Printf("decoder.Decode: %+v", err)
		log.Printf("Original response: %+v", res.Body)
		return
	}

	log.Printf("Creating new executable script")

	file, err := os.CreateTemp("", "remote-client-*.sh")
	if err != nil {
		log.Printf("os.CreateTemp: %+v", err)
	}
	defer file.Close()
	defer os.Remove(file.Name())
	file.Chmod(0766)

	data := []byte(serverResponse.Body)
	if _, err := file.Write(data); err != nil {
		log.Printf("file.Write: %+v", err)
	}

	log.Printf("Script name: %+v. Executing script...", file.Name())

	var outbuf, errbuf bytes.Buffer
	cmd := exec.Command("sh", file.Name())
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf

	err = cmd.Run()
	stdout := outbuf.String()
	stderr := errbuf.String()
	var exitCode int

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			exitCode = ws.ExitStatus()
		} else {
			log.Printf("Could not get exit code")
			exitCode = 9999
			if stderr == "" {
				stderr = err.Error()
			}
		}
	} else {
		ws := cmd.ProcessState.Sys().(syscall.WaitStatus)
		exitCode = ws.ExitStatus()
	}

	serverRequest := ServerRequest{Status: strconv.Itoa(exitCode), Result: fmt.Sprintf("stdout: %sstderr:%s", stdout, stderr)}
	serverRequestJson, err := json.Marshal(serverRequest)
	if err != nil {
		log.Printf("json.Marshal: %+v", err)
		log.Printf("Original request: %+v", serverRequest)
		return
	}

	req, err = http.NewRequest("PATCH", fmt.Sprintf("%s/api/v1/clients/me/commands/%s", BACK_OFFICE_URL, serverResponse.Id), bytes.NewBuffer(serverRequestJson))
	req.Header = http.Header{
		"accept":        {"*/*"},
		"Authorization": {fmt.Sprintf("Basic %s", BACK_OFFICE_API_KEY)},
		"Content-Type":  {"application/json"},
	}

	res, err = client.Do(req)
	if err != nil {
		log.Printf("client.Do: %+v", err)
		return
	}

	log.Printf("Successfully executed http request. Status code was: %+v", res.StatusCode)

	if res.StatusCode != http.StatusOK {
		return
	}

	next_call_index = -1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
