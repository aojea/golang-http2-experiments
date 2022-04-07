// Based on https://github.com/golang/build/blob/master/revdial/v2/revdial.go

// Package revdial implements a Dialer and Listener which work together
// to turn an accepted connection (for instance, a Hijacked HTTP request) into
// a Dialer which can then create net.Conns connecting back to the original
// dialer, which then gets a net.Listener accepting those conns.
//
// This is basically a very minimal SOCKS5 client & server.
//
// The motivation is that sometimes you want to run a server on a
// machine deep inside a NAT. Rather than connecting to the machine
// directly (which you can't, because of the NAT), you have the
// sequestered machine connect out to a public machine. Both sides
// then use revdial and the public machine can become a client for the
// NATed machine.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// dialerUniqParam is the parameter name of the GET URL form value
// containing the Dialer's random unique ID.
const dialerUniqParam = "revdial.dialer"

// The Dialer can create new connections back to the origin
type Dialer struct {
	dmapMu       sync.Mutex
	incomingConn map[string]chan net.Conn
	connReady    chan bool
	donec        chan struct{}
	closeOnce    sync.Once
}

// NewDialer returns the side of the connection which will initiate
// new connections.
// The connPath is the HTTP path and optional query (but
// without scheme or host) on the dialer where the ConnHandler is
// mounted.
func NewDialer() *Dialer {
	d := &Dialer{
		donec:     make(chan struct{}),
		connReady: make(chan bool),
		// rev connection create a new back connection
		// channel id = remote address
		incomingConn: make(map[string]chan net.Conn),
	}

	return d
}

// Done returns a channel which is closed when d is closed (either by
// this process on purpose, by a local error, or close or error from
// the peer).
func (d *Dialer) Done() <-chan struct{} { return d.donec }

// Close closes the Dialer.
func (d *Dialer) Close() error {
	d.closeOnce.Do(d.close)
	return nil
}

func (d *Dialer) close() {
	d.incomingConn = nil
	close(d.donec)
}

// Dial creates a new connection back to the Listener using a reverse tunnel/
// addr is the uniq id passed as parameter when the reverse connection is created
func (d *Dialer) Dial(ctx context.Context, network string, addr string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(time.Second*5))
	defer cancel()
	log.Printf("Dialing %s %s", network, addr)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	// pick up one connection:
	select {
	case c := <-d.incomingConn[host]:
		return c, nil
	case <-d.donec:
		return nil, errors.New("revdial.Dialer closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ServeHTTP is the HTTP handler that needs to be mounted somewhere
// that the Listeners can dial out and get to. A dialer to connect to it
// is given to NewListener and the path to reach it is given to NewDialer
// to use in messages to the listener.
func (d *Dialer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil {
		http.Error(w, "only TLS supported", http.StatusInternalServerError)
		return
	}
	if r.Proto != "HTTP/2.0" {
		http.Error(w, "only HTTP/2.0 supported", http.StatusHTTPVersionNotSupported)
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		http.Error(w, "expected GET request to revdial conn handler", http.StatusMethodNotAllowed)
		return
	}

	// TODO add additional id , right now only the remote IP identifies the connections
	dialerUniq := r.FormValue(dialerUniqParam)
	if _, ok := d.incomingConn[dialerUniq]; !ok {
		d.incomingConn[dialerUniq] = make(chan net.Conn)
	}

	log.Printf("created reverse connection to %s %s id %s", r.RequestURI, r.RemoteAddr, dialerUniq)
	// First flash response headers
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	conn := NewConn(r.Body, flushWriter{w})
	select {
	case d.incomingConn[dialerUniq] <- conn:
	case <-d.donec:
		http.Error(w, "Reverse dialer closed", http.StatusInternalServerError)
		return
	}
	// keep the handler alive until the connection is closed
	<-conn.Done()
	log.Printf("Conn from %s done", r.RemoteAddr)
}

func (d *Dialer) ProxyRequestHandler() func(http.ResponseWriter, *http.Request) {
	tr := http.DefaultTransport.(*http.Transport)
	tr.DialContext = d.Dial
	client := http.Client{}
	client.Transport = tr
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("proxy req %s from %s", r.RequestURI, r.RemoteAddr)
		req := &http.Request{}
		req.URL = &url.URL{}
		req.URL.Path = r.URL.Path
		req.URL.Host = "revdial1"
		req.URL.Scheme = "http"
		res, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		defer res.Body.Close()
		// copy request body to next request body
		// copy response body to the writer
		io.Copy(flushWriter{w}, res.Body)
		log.Printf("proxy server closed %s ", "")
	}
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

func (fw flushWriter) Close() error {
	return nil
}

// NewListener returns a new Listener, accepting connections which
// arrive from the provided server connection, which should be after
// any necessary authentication (usually after an HTTP exchange).
//
// The provided dialServer func is responsible for connecting back to
// the server and doing TLS setup.
func NewListener(client *http.Client, url string) *Listener {
	ln := &Listener{
		url:    url, // https://host:port/revdial
		client: client,
		connc:  make(chan net.Conn, 8), // arbitrary
		donec:  make(chan struct{}),
	}
	go ln.run()
	return ln
}

var _ net.Listener = (*Listener)(nil)

// Listener is a net.Listener, returning new connections which arrive
// from a corresponding Dialer.
type Listener struct {
	url    string
	client *http.Client
	connc  chan net.Conn
	donec  chan struct{}

	mu      sync.Mutex // guards below, closing connc, and writing to rw
	readErr error
	closed  bool
}

// run reads control messages from the public server forever until the connection dies, which
// then closes the listener.
func (ln *Listener) run() {
	defer ln.Close()

	// Create connections
	for {
		pr, pw := io.Pipe()
		url := ln.url + "?" + dialerUniqParam + "=" + "revdial1"
		req, err := http.NewRequest("GET", url, pr)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Listener creating connection to %s", url)
		res, err := ln.client.Do(req)
		if err != nil {
			log.Fatal(err)
		}

		c := NewConn(res.Body, pw)
		select {
		case ln.connc <- c:
		case <-ln.donec:
		}
		<-c.Done()
		log.Printf("Listener connection to %s closed", url)

	}
}

// Closed reports whether the listener has been closed.
func (ln *Listener) Closed() bool {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	return ln.closed
}

// Accept blocks and returns a new connection, or an error.
func (ln *Listener) Accept() (net.Conn, error) {
	c, ok := <-ln.connc
	if !ok {
		ln.mu.Lock()
		err, closed := ln.readErr, ln.closed
		ln.mu.Unlock()
		if err != nil && !closed {
			return nil, fmt.Errorf("revdial: Listener closed; %v", err)
		}
		return nil, ErrListenerClosed
	}
	return c, nil
}

// ErrListenerClosed is returned by Accept after Close has been called.
var ErrListenerClosed = errors.New("revdial: Listener closed")

// Close closes the Listener, making future Accept calls return an
// error.
func (ln *Listener) Close() error {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	if ln.closed {
		return nil
	}
	ln.closed = true
	close(ln.connc)
	close(ln.donec)
	return nil
}

// Addr returns a dummy address. This exists only to conform to the
// net.Listener interface.
func (ln *Listener) Addr() net.Addr { return connAddr{} }
