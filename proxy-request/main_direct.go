package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

func direct() {

	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("backend: revc req %s %s", r.RequestURI, r.RemoteAddr)
		fmt.Fprintf(w, "Hello, %s I'm backend serve %s", r.RemoteAddr, strings.Repeat("asd", 1000))
	}))
	backend.EnableHTTP2 = true
	backend.StartTLS()
	defer backend.Close()
	log.Println("backend url", backend.URL)

	time.Sleep(1 * time.Second)
	// proxy server forwards to backend
	proxyServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("proxy server: revc req %s %s", r.RequestURI, r.RemoteAddr)
		// First flash response headers
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		pr, pw := io.Pipe()
		defer pw.Close()
		client := backend.Client()
		url := backend.URL
		log.Printf("proxy client: send req %s ", url)
		req, err := http.NewRequest("GET", url, pr)
		if err != nil {
			log.Fatal(err)
		}
		res, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		defer res.Body.Close()
		// copy request body to next request body
		// copy response body to the writer
		io.Copy(flushWriter{w}, res.Body)
		log.Printf("proxy server closed %s ", url)

	}))
	proxyServer.EnableHTTP2 = true
	proxyServer.StartTLS()
	defer proxyServer.Close()

	//
	proxyClient := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("proxy client: revc req %s %s", r.RequestURI, r.RemoteAddr)
		// First flash response headers
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		client := proxyServer.Client()
		url := proxyServer.URL
		log.Printf("proxy client: send req %s ", url)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			log.Fatal(err)
		}
		res, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		defer res.Body.Close()
		// copy request body to next request body
		// copy response body to the writer
		io.Copy(flushWriter{w}, res.Body)
		log.Printf("proxy client closed")

	}))
	proxyClient.EnableHTTP2 = true
	proxyClient.StartTLS()
	defer proxyClient.Close()

	// final

	hc := proxyClient.Client()
	log.Printf("client: send final req")

	resp, err := hc.Get(proxyClient.URL + "/proxy-request/pods")
	if err != nil {
		log.Fatalf("Request Failed: %s", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Reading body failed: %s", err)
	}
	// Log the request body
	bodyString := string(body)
	log.Print(bodyString)
	time.Sleep(1 * time.Second)

}
