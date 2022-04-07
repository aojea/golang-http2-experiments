package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

func proxyGet(client *http.Client, url string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("revc req %s %s", r.RequestURI, r.RemoteAddr)
		// First flash response headers
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		log.Printf("proxy client: send req %s ", url)
		req, err := http.NewRequest("GET", url, nil)
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
	})
}

// Send HTML and push image
func handlerHtml(w http.ResponseWriter, r *http.Request) {
	pusher, ok := w.(http.Pusher)
	if ok {
		fmt.Println("Push /image")
		pusher.Push("/image", nil)
	}
	w.Header().Add("Content-Type", "text/html")
	fmt.Fprintf(w, `<html><body><img src="/image"></body></html>`)
}

func main() {

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
	mux := http.NewServeMux()
	mux.Handle("/", proxyGet(backend.Client(), backend.URL))
	proxyServer := httptest.NewUnstartedServer(mux)
	proxyServer.EnableHTTP2 = true
	proxyServer.StartTLS()
	defer proxyServer.Close()

	//
	mux = http.NewServeMux()
	mux.Handle("/", proxyGet(proxyServer.Client(), proxyServer.URL))
	proxyClient := httptest.NewUnstartedServer(mux)
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

type flushWriter struct {
	w io.Writer
}

func (fw flushWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return
}

var _ net.Conn = (*conn)(nil)

type conn struct {
	r  io.ReadCloser
	wc io.WriteCloser
}

// Write writes data to the connection
func (c *conn) Write(data []byte) (int, error) {
	return c.wc.Write(data)
}

// Read reads data from the connection
func (c *conn) Read(data []byte) (int, error) {
	return c.r.Read(data)
}

// Close closes the connection
func (c *conn) Close() error {
	c.r.Close()
	return c.wc.Close()
}

func (c *conn) LocalAddr() net.Addr {
	return mockAddr{}
}

func (c *conn) RemoteAddr() net.Addr {
	return mockAddr{}
}

func (c *conn) SetDeadline(t time.Time) error {
	panic("not implemented")
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	panic("not implemented")
}

func (c *conn) SetReadDeadline(t time.Time) error {
	panic("not implemented")
}

type mockAddr struct{}

func (mockAddr) Network() string { return "mock" }
func (mockAddr) String() string  { return "mock" }
