package resolver

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

const maxPacketSize = 1024

// packetHandlerFn set
type packetHandlerFn func(b []byte) []byte

// hairpin creates a synchronous, in-memory, packet network connection
// implementing the Conn interface. Reads on the connection are matched
// with writes. Packets are processed by the provided hook, if exist,
// and copied directly; there is no internal buffering.
type hairpin struct {
	wrMu sync.Mutex // Serialize Write operations

	readCh chan []byte // Used to communicate Write and Read

	once sync.Once // Protects closing the connection
	done chan struct{}

	readDeadline  connDeadline
	writeDeadline connDeadline

	localAddr  net.Addr
	remoteAddr net.Addr

	// hook for processing packets
	// nil means packet are copied directly
	packetHandler packetHandlerFn
}

// implement PacketConn interface
var _ net.PacketConn = &hairpin{}

func Hairpin(fn packetHandlerFn) *hairpin {
	return &hairpin{
		readCh:        make(chan []byte, maxPacketSize),
		done:          make(chan struct{}),
		readDeadline:  makeConnDeadline(),
		writeDeadline: makeConnDeadline(),
		packetHandler: fn,
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

type hairpinAddress struct {
	addr string
}

func (h hairpinAddress) Network() string {
	if h.addr != "" {
		return h.addr
	}
	return "Hairpin"
}
func (h hairpinAddress) String() string {
	if h.addr != "" {
		return h.addr
	}
	return "Hairpin"
}

func (h *hairpin) SetLocalAddr(addr net.Addr) {
	h.localAddr = addr
}
func (h *hairpin) SetRemoteAddr(addr net.Addr) {
	h.remoteAddr = addr
}

func (h *hairpin) LocalAddr() net.Addr {
	if h.localAddr != nil {
		return h.localAddr
	}
	return hairpinAddress{}
}
func (h *hairpin) RemoteAddr() net.Addr {
	if h.remoteAddr != nil {
		return h.remoteAddr
	}
	return hairpinAddress{}
}

func (h *hairpin) Read(b []byte) (int, error) {
	n, _, err := h.ReadFrom(b)
	return n, err
}
func (h *hairpin) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := h.read(b)
	if err != nil && err != io.EOF && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "read", Net: "Hairpin", Err: err}
	}
	return n, hairpinAddress{}, err
}

func (h *hairpin) read(b []byte) (n int, err error) {
	switch {
	case isClosedChan(h.done):
		return 0, io.ErrClosedPipe
	case isClosedChan(h.readDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	}

	select {
	case bw := <-h.readCh:
		output := h.packetHandler(bw)
		// nil means the server is closing the connection
		if output == nil {
			return 0, io.EOF
		}
		nr := copy(b, output)
		return nr, nil
	case <-h.done:
		return 0, io.EOF
	case <-h.readDeadline.wait():
		return 0, os.ErrDeadlineExceeded
	}
}

func (h *hairpin) Write(b []byte) (int, error) {
	return h.WriteTo(b, hairpinAddress{})
}

func (h *hairpin) WriteTo(b []byte, _ net.Addr) (int, error) {
	n, err := h.write(b)
	if err != nil && err != io.ErrClosedPipe {
		err = &net.OpError{Op: "write", Net: "Hairpin", Err: err}
	}
	return n, err
}

func (h *hairpin) write(b []byte) (n int, err error) {
	if len(b) > maxPacketSize {
		return 0, syscall.EMSGSIZE
	}

	switch {
	case isClosedChan(h.done):
		return 0, io.ErrClosedPipe
	case isClosedChan(h.writeDeadline.wait()):
		return 0, os.ErrDeadlineExceeded
	case h.packetHandler == nil:
		return n, io.ErrClosedPipe
	}

	select {
	case <-h.done:
		return n, io.ErrClosedPipe
	case <-h.writeDeadline.wait():
		return n, os.ErrDeadlineExceeded
	default:
	}

	// Copy the buffer and ensure entirety of b is written together
	// the process handler will access the buffer so it can mutate
	// the input.
	h.wrMu.Lock()
	defer h.wrMu.Unlock()
	packet := make([]byte, len(b))
	nr := copy(packet, b)
	h.readCh <- packet
	if nr != len(b) {
		return nr, io.ErrShortWrite
	}
	return len(b), nil
}

func (h *hairpin) SetDeadline(t time.Time) error {
	if isClosedChan(h.done) {
		return io.ErrClosedPipe
	}
	h.readDeadline.set(t)
	h.writeDeadline.set(t)
	return nil
}

func (h *hairpin) SetReadDeadline(t time.Time) error {
	if isClosedChan(h.done) {
		return io.ErrClosedPipe
	}
	h.readDeadline.set(t)
	return nil
}

func (h *hairpin) SetWriteDeadline(t time.Time) error {
	if isClosedChan(h.done) {
		return io.ErrClosedPipe
	}
	h.writeDeadline.set(t)
	return nil
}

func (l *hairpin) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

// Dialer
type HairpinDialer struct {
	// PacketHandler must be safe to call concurrently
	PacketHandler packetHandlerFn
}

// Dial creates an in memory connection that is processed by the packet handler
func (h *HairpinDialer) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	// Hairpin implements net.Conn interface
	conn := Hairpin(h.PacketHandler)
	conn.SetRemoteAddr(hairpinAddress{
		addr: address,
	})
	return conn, nil
}

// Listener
type HairpinListener struct {
	connPool []net.Conn
	address  string

	PacketHandler packetHandlerFn
}

var _ net.Listener = &HairpinListener{}

func (h *HairpinListener) Accept() (net.Conn, error) {
	// Hairpin implements net.Conn interface
	conn := Hairpin(h.PacketHandler)
	conn.SetLocalAddr(hairpinAddress{
		addr: h.address,
	})
	return conn, nil
}

func (h *HairpinListener) Close() error {
	var aggError error
	for _, c := range h.connPool {
		if err := c.Close(); err != nil {
			aggError = fmt.Errorf("%w", err)
		}
	}
	return aggError
}

func (h *HairpinListener) Addr() net.Addr {
	return hairpinAddress{
		addr: h.address,
	}
}

func (h *HairpinListener) Listen(network, address string) (net.Listener, error) {
	h.address = address
	return h, nil
}
