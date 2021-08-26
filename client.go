package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

const url = "https://localhost:8443"

var httpVersion = flag.Int("version", 2, "HTTP version")

func main() {
	flag.Parse()
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	tr := http.DefaultTransport
	tr.(*http.Transport).TLSClientConfig = tlsConfig
	//tr.(*http.Transport).MaxIdleConnsPerHost = -1

	client := &http.Client{
		Transport: tr,
	}

	for i := 1; i < 5; i++ {
		// Perform the request
		func() {
			resp, err := client.Get(url)
			if err != nil {
				log.Fatalf("Failed get: %s", err)
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Fatalf("Failed reading response body: %s", err)
			}
			fmt.Printf(
				"Got response %d: %s %s\n",
				resp.StatusCode, resp.Proto, string(body))
		}()
		tr.(*http.Transport).CloseIdleConnections()
	}
}
