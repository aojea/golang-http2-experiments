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

type packetHandlerFn func(b []byte) []byte

// MemoryConn creates an in-memory network connection
// Writes are sent to a custom function that process
// the packets.
// Reads read the output of the custom function.
type MemoryConn struct {
	readCh chan []byte

	once sync.Once
	done chan struct{}

	readDeadline  connDeadline
	writeDeadline connDeadline

	localAddr  net.Addr
	remoteAddr net.Addr

	// PacketHandler must be safe to call concurrently
	PacketHandler packetHandlerFn
}

var _ net.PacketConn = &MemoryConn{}

func NewMemoryConn(fn packetHandlerFn) *MemoryConn {
	return &MemoryConn{
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

type MemoryConnAddress struct {
	addr string
}

func (m MemoryConnAddress) Network() string {
	if m.addr != "" {
		return m.addr
	}
	return "MemoryConn"
}
func (m MemoryConnAddress) String() string {
	if m.addr != "" {
		return m.addr
	}
	return "MemoryConn"
}

func (l *MemoryConn) SetLocalAddr(addr net.Addr) {
	l.localAddr = addr
}
func (l *MemoryConn) SetRemoteAddr(addr net.Addr) {
	l.remoteAddr = addr
}

func (l *MemoryConn) LocalAddr() net.Addr {
	if l.localAddr != nil {
		return l.localAddr
	}
	return MemoryConnAddress{}
}
func (l *MemoryConn) RemoteAddr() net.Addr {
	if l.remoteAddr != nil {
		return l.remoteAddr
	}
	return MemoryConnAddress{}
}

func (l *MemoryConn) Read(b []byte) (int, error) {
	n, _, err := l.ReadFrom(b)
	return n, err
}
func (l *MemoryConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := l.read(b)
	if err != nil && err != io.EOF && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "read", Net: "MemoryConn", Err: err}
	}
	return n, MemoryConnAddress{}, err
}

func (l *MemoryConn) read(b []byte) (n int, err error) {
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

func (l *MemoryConn) Write(b []byte) (int, error) {
	return l.WriteTo(b, MemoryConnAddress{})
}

func (l *MemoryConn) WriteTo(b []byte, _ net.Addr) (int, error) {
	n, err := l.write(b)
	if err != nil && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "write", Net: "MemoryConn", Err: err}
	}
	return n, err
}

func (l *MemoryConn) write(b []byte) (n int, err error) {
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

func (l *MemoryConn) SetDeadline(t time.Time) error {
	if isClosedChan(l.done) {
		return io.ErrClosedPipe
	}
	l.readDeadline.set(t)
	l.writeDeadline.set(t)
	return nil
}

func (l *MemoryConn) SetReadDeadline(t time.Time) error {
	if isClosedChan(l.done) {
		return io.ErrClosedPipe
	}
	l.readDeadline.set(t)
	return nil
}

func (l *MemoryConn) SetWriteDeadline(t time.Time) error {
	if isClosedChan(l.done) {
		return io.ErrClosedPipe
	}
	l.writeDeadline.set(t)
	return nil
}

func (l *MemoryConn) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

// Dialer
type MemoryDialer struct {
	// PacketHandler must be safe to call concurrently
	PacketHandler packetHandlerFn
}

// Dial creates an in memory connection that is processed by the packet handler
func (m *MemoryDialer) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	// MemoryConn implements net.Conn interface
	conn := NewMemoryConn(m.PacketHandler)
	conn.SetRemoteAddr(MemoryConnAddress{
		addr: address,
	})
	return conn, nil
}

// Listener
type MemoryListener struct {
	connPool []net.Conn
	address  string

	PacketHandler packetHandlerFn
}

var _ net.Listener = &MemoryListener{}

func (m *MemoryListener) Accept() (net.Conn, error) {
	// MemoryConn implements net.Conn interface
	conn := NewMemoryConn(m.PacketHandler)
	conn.SetLocalAddr(MemoryConnAddress{
		addr: m.address,
	})
	return conn, nil
}

func (m *MemoryListener) Close() error {
	var aggError error
	for _, c := range m.connPool {
		if err := c.Close(); err != nil {
			aggError = fmt.Errorf("%w", err)
		}
	}
	return aggError
}

func (m *MemoryListener) Addr() net.Addr {
	return MemoryConnAddress{
		addr: m.address,
	}
}

func (m *MemoryListener) Listen(network, address string) (net.Listener, error) {
	m.address = address
	return m, nil
}
