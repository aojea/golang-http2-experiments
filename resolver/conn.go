package resolver

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// connection parameters (copied from net.Pipe)
// https://cs.opensource.google/go/go/+/refs/tags/go1.17:src/net/pipe.go;bpv=0;bpt=1

// connDeadline is an abstraction for handling timeouts.
type connDeadline struct {
	mu     sync.Mutex // Guards timer and cancel
	timer  *time.Timer
	cancel chan struct{} // Must be non-nil
}

func makeConnDeadline() connDeadline {
	return connDeadline{cancel: make(chan struct{})}
}

// set sets the point in time when the deadline will time out.
// A timeout event is signaled by closing the channel returned by waiter.
// Once a timeout has occurred, the deadline can be refreshed by specifying a
// t value in the future.
//
// A zero value for t prevents timeout.
func (d *connDeadline) set(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil && !d.timer.Stop() {
		<-d.cancel // Wait for the timer callback to finish and close cancel
	}
	d.timer = nil

	// Time is zero, then there is no deadline.
	closed := isClosedChan(d.cancel)
	if t.IsZero() {
		if closed {
			d.cancel = make(chan struct{})
		}
		return
	}

	// Time in the future, setup a timer to cancel in the future.
	if dur := time.Until(t); dur > 0 {
		if closed {
			d.cancel = make(chan struct{})
		}
		d.timer = time.AfterFunc(dur, func() {
			close(d.cancel)
		})
		return
	}

	// Time in the past, so close immediately.
	if !closed {
		close(d.cancel)
	}
}

// wait returns a channel that is closed when the deadline is exceeded.
func (d *connDeadline) wait() chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cancel
}

func isClosedChan(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

type inMemoryDNSAddress struct{}

func (inMemoryDNSAddress) Network() string { return "inMemoryDNS" }
func (inMemoryDNSAddress) String() string  { return "inMemoryDNS" }

// Dial creates an in memory connection
func (r *InMemoryDNS) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	// InMemoryDNS implements net.Conn interface
	return r, nil
}

func (r *InMemoryDNS) LocalAddr() net.Addr  { return inMemoryDNSAddress{} }
func (r *InMemoryDNS) RemoteAddr() net.Addr { return inMemoryDNSAddress{} }

func (r *InMemoryDNS) Read(b []byte) (int, error) {
	n, _, err := r.ReadFrom(b)
	return n, err
}
func (r *InMemoryDNS) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := r.read(b)
	if err != nil && err != io.EOF && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "read", Net: "inMemoryDNS", Err: err}
	}
	return n, inMemoryDNSAddress{}, err
}

func (r *InMemoryDNS) read(b []byte) (n int, err error) {
	switch {
	case isClosedChan(r.done):
		return 0, io.ErrClosedPipe
	case isClosedChan(r.readDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	}

	select {
	case bw := <-r.readCh:
		nr := copy(b, bw)
		fmt.Println("READ bytes", bw)
		return nr, nil
	case <-r.done:
		return 0, io.EOF
	case <-r.readDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	}
}

func (r *InMemoryDNS) Write(b []byte) (int, error) {
	return r.WriteTo(b, inMemoryDNSAddress{})
}

func (r *InMemoryDNS) WriteTo(b []byte, _ net.Addr) (int, error) {
	n, err := r.write(b)
	if err != nil && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "write", Net: "inMemoryDNS", Err: err}
	}
	return n, err
}

func (r *InMemoryDNS) write(b []byte) (n int, err error) {
	switch {
	case isClosedChan(r.done):
		return 0, io.ErrClosedPipe
	case isClosedChan(r.writeDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	}

	select {
	case <-r.done:
		return n, io.ErrClosedPipe
	case <-r.writeDeadline.wait():
		return n, os.ErrDeadlineExceeded
	default:
	}
	go r.processDNSRequest(b)
	return len(b), nil
}

func (r *InMemoryDNS) SetDeadline(t time.Time) error {
	if isClosedChan(r.done) {
		return io.ErrClosedPipe
	}
	r.readDeadline.set(t)
	r.writeDeadline.set(t)
	return nil
}

func (r *InMemoryDNS) SetReadDeadline(t time.Time) error {
	if isClosedChan(r.done) {
		return io.ErrClosedPipe
	}
	r.readDeadline.set(t)
	return nil
}

func (r *InMemoryDNS) SetWriteDeadline(t time.Time) error {
	if isClosedChan(r.done) {
		return io.ErrClosedPipe
	}
	r.writeDeadline.set(t)
	return nil
}

func (r *InMemoryDNS) Close() error {
	r.once.Do(func() { close(r.done) })
	return nil
}
