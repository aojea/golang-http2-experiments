package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"
)

func main() {

	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("backend: revc req %s %s", r.RequestURI, r.RemoteAddr)
		fmt.Fprintf(w, "Hello, %s I'm backend serve", r.RemoteAddr)
	}))
	backend.EnableHTTP2 = true
	backend.StartTLS()
	defer backend.Close()
	log.Println("backend url", backend.URL)

	time.Sleep(1 * time.Second)
	// proxy server forwards to backend
	proxyServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("proxy server: revc req %s %s", r.RequestURI, r.RemoteAddr)
		host := r.URL.Query().Get("host")
		if host == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// First flash response headers
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		u, err := url.Parse(host)
		if err != nil {
			panic(err)
		}
		log.Printf("proxy server: dial %s ", u.Host)

		conn, err := net.Dial("tcp", u.Host)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		defer conn.Close()
		// copy request body to next request body
		go io.Copy(flushWriter{w}, conn)
		io.Copy(conn, r.Body)

		log.Printf("proxy server closed %s ", u.Host)

	}))
	proxyServer.EnableHTTP2 = true
	proxyServer.StartTLS()
	defer proxyServer.Close()

	//
	proxyClient := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("proxy client: revc req %s %s", r.RequestURI, r.RemoteAddr)
		host := r.URL.Query().Get("host")
		if host == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// First flash response headers
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		pr, pw := io.Pipe()
		defer pw.Close()
		client := proxyServer.Client()
		url := proxyServer.URL + "?host=" + host
		log.Printf("proxy client: send req %s ", url)
		req, err := http.NewRequest("PUT", url, pr)
		if err != nil {
			log.Fatal(err)
		}
		res, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		defer res.Body.Close()
		// copy request body to next request body
		go io.Copy(pw, r.Body)
		// copy response body to the writer
		io.Copy(flushWriter{w}, res.Body)
		log.Printf("proxy client closed")

	}))
	proxyClient.EnableHTTP2 = true
	proxyClient.StartTLS()
	defer proxyClient.Close()

	// final
	tr := backend.Client().Transport.(*http.Transport)
	tr.Dial = func(network string, addr string) (net.Conn, error) {
		log.Printf("client: dial final req %s %s", network, addr)
		pr, pw := io.Pipe()
		client := proxyClient.Client()
		url := proxyClient.URL + "?host=" + backend.URL
		req, err := http.NewRequest("PUT", url, pr)
		if err != nil {
			log.Fatal(err)
		}
		var c *conn
		log.Printf("client: send req %s", url)
		res, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("client: create conn %s", url)

		c = &conn{
			r:  res.Body,
			wc: pw,
		}
		return c, nil
	}

	hc := http.Client{
		Transport: tr,
	}
	log.Printf("client: send final req")

	resp, err := hc.Get("https://example.com/todos/1")
	if err != nil {
		log.Fatalf("Request Failed: %s", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Reading body failed: %s", err)
	}
	// Log the request body
	bodyString := string(body)
	log.Print(bodyString)
	resp.Body.Close()
	tr.CloseIdleConnections()
	backend.Close()

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
