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

// LocalDialer creates an in-memory network connection
// Writes are sent to a custom function that process
// the packets.
// Reads read the output of the custom function.
type LocalDialer struct {
	readCh chan []byte

	once sync.Once
	done chan struct{}

	readDeadline  connDeadline
	writeDeadline connDeadline

	// PacketHandler must be safe to call concurrently
	PacketHandler func(b []byte) []byte
}

var _ net.PacketConn = &LocalDialer{}

func NewLocalDialer(fn func(b []byte) []byte) *LocalDialer {
	return &LocalDialer{
		readCh:        make(chan []byte),
		done:          make(chan struct{}),
		readDeadline:  makeConnDeadline(),
		writeDeadline: makeConnDeadline(),
		PacketHandler: fn,
	}

}

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

type localDialerAddress struct{}

func (localDialerAddress) Network() string { return "localDialer" }
func (localDialerAddress) String() string  { return "localDialer" }

// Dial creates an in memory connection that is processed by the packet handler
func (l *LocalDialer) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	// localDialer implements net.Conn interface
	return l, nil
}

func (l *LocalDialer) LocalAddr() net.Addr  { return localDialerAddress{} }
func (l *LocalDialer) RemoteAddr() net.Addr { return localDialerAddress{} }

func (l *LocalDialer) Read(b []byte) (int, error) {
	n, _, err := l.ReadFrom(b)
	return n, err
}
func (l *LocalDialer) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := l.read(b)
	if err != nil && err != io.EOF && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "read", Net: "localDialer", Err: err}
	}
	return n, localDialerAddress{}, err
}

func (l *LocalDialer) read(b []byte) (n int, err error) {
	switch {
	case isClosedChan(l.done):
		return 0, io.ErrClosedPipe
	case isClosedChan(l.readDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	}

	select {
	case bw := <-l.readCh:
		nr := copy(b, bw)
		fmt.Println("READ bytes", bw)
		return nr, nil
	case <-l.done:
		return 0, io.EOF
	case <-l.readDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	}
}

func (l *LocalDialer) Write(b []byte) (int, error) {
	return l.WriteTo(b, localDialerAddress{})
}

func (l *LocalDialer) WriteTo(b []byte, _ net.Addr) (int, error) {
	n, err := l.write(b)
	if err != nil && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "write", Net: "localDialer", Err: err}
	}
	return n, err
}

func (l *LocalDialer) write(b []byte) (n int, err error) {
	switch {
	case isClosedChan(l.done):
		return 0, io.ErrClosedPipe
	case isClosedChan(l.writeDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	case l.PacketHandler == nil:
		return n, io.ErrClosedPipe
	}

	select {
	case <-l.done:
		return n, io.ErrClosedPipe
	case <-l.writeDeadline.wait():
		return n, os.ErrDeadlineExceeded
	default:
	}

	// TODO bound this and allow to timeout
	go func() {
		l.readCh <- l.PacketHandler(b)
	}()

	return len(b), nil
}

func (l *LocalDialer) SetDeadline(t time.Time) error {
	if isClosedChan(l.done) {
		return io.ErrClosedPipe
	}
	l.readDeadline.set(t)
	l.writeDeadline.set(t)
	return nil
}

func (l *LocalDialer) SetReadDeadline(t time.Time) error {
	if isClosedChan(l.done) {
		return io.ErrClosedPipe
	}
	l.readDeadline.set(t)
	return nil
}

func (l *LocalDialer) SetWriteDeadline(t time.Time) error {
	if isClosedChan(l.done) {
		return io.ErrClosedPipe
	}
	l.writeDeadline.set(t)
	return nil
}

func (l *LocalDialer) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}
