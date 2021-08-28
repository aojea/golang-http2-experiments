package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/aojea/golang-http2-experiments/resolver"
)

const url = "https://www.google.es:8443"

var httpVersion = flag.Int("version", 2, "HTTP version")

func main() {
	flag.Parse()

	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Resolver:  resolver.NewInMemoryResolver(&resolver.DNSShim{}),
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		TLSClientConfig:       tlsConfig,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

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
	}
}
