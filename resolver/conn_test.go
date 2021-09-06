// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// ref: https://github.com/golang/net/blob/master/nettest/conntest.go
package resolver

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

var (
	aLongTimeAgo = time.Unix(233431200, 0)
	neverTimeout = time.Time{}
)

// TestBasicIO tests that the data sent on c is properly received back on c.
func TestBasicIO(t *testing.T) {
	ph := func(b []byte) []byte {
		return b
	}
	c := NewMemoryConn(ph)
	want := make([]byte, 1<<20)
	rand.New(rand.NewSource(0)).Read(want)
	dataCh := make(chan []byte)

	go func() {
		rd := bytes.NewReader(want)
		if err := chunkedCopy(c, rd); err != nil {
			t.Errorf("unexpected buffer Write error: %v", err)
		}
	}()

	go func() {
		wr := new(bytes.Buffer)
		if err := chunkedCopy(wr, c); err != nil {
			t.Errorf("unexpected buffer Read error: %v", err)
		}

		dataCh <- wr.Bytes()
	}()

	// TODO: wait until all data is processed
	// closing the connection close the reader too
	time.Sleep(1 * time.Second)
	if err := c.Close(); err != nil {
		t.Errorf("unexpected c.Close error: %v", err)
	}
	if got := <-dataCh; !bytes.Equal(got, want) {
		t.Error("transmitted data differs")
	}

}

// testPingPong tests that the two endpoints can synchronously send data to
// each other in a typical request-response pattern.
func TestPingPong(t *testing.T) {
	var prev uint64
	ph := func(buf []byte) []byte {
		v := binary.LittleEndian.Uint64(buf)
		binary.LittleEndian.PutUint64(buf, v+1)
		if prev != 0 && prev+2 != v {
			t.Errorf("mismatching value: got %d, want %d", v, prev+2)
		}
		prev = v
		fmt.Println("debug ping", v)
		if v == 1000 {
			return []byte{}
		}
		return buf
	}
	c := NewMemoryConn(ph)

	var wg sync.WaitGroup
	defer wg.Wait()

	pingPonger := func(c net.Conn) {
		defer wg.Done()
		buf := make([]byte, 8)
		var prev uint64
		for {
			if _, err := io.ReadFull(c, buf); err != nil {
				if err == io.EOF {
					break
				}
				t.Errorf("unexpected Read error: %v", err)
			}

			v := binary.LittleEndian.Uint64(buf)
			binary.LittleEndian.PutUint64(buf, v+1)
			if prev != 0 && prev+2 != v {
				t.Errorf("mismatching value: got %d, want %d", v, prev+2)
			}
			prev = v
			fmt.Println("debug pong", v)

			if v == 1000 {
				break
			}

			if _, err := c.Write(buf); err != nil {
				t.Errorf("unexpected Write error: %v", err)
				break
			}
		}
		if err := c.Close(); err != nil {
			t.Errorf("unexpected Close error: %v", err)
		}
	}

	wg.Add(1)
	go pingPonger(c)

	// Start off the chain reaction.
	if _, err := c.Write(make([]byte, 8)); err != nil {
		t.Errorf("unexpected c.Write error: %v", err)
	}
}

// TestRacyRead tests that it is safe to mutate the input Read buffer
// immediately after cancelation has occurred.
func TestRacyRead(t *testing.T) {
	ph := func(b []byte) []byte {
		return b
	}
	c := NewMemoryConn(ph)
	go chunkedCopy(c, rand.New(rand.NewSource(0)))

	var wg sync.WaitGroup
	defer wg.Wait()

	c.SetReadDeadline(time.Now().Add(time.Millisecond))
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			b1 := make([]byte, 1024)
			b2 := make([]byte, 1024)
			for j := 0; j < 100; j++ {
				_, err := c.Read(b1)
				copy(b1, b2) // Mutate b1 to trigger potential race
				if err != nil {
					checkForTimeoutError(t, err)
					c.SetReadDeadline(time.Now().Add(time.Millisecond))
				}
			}
		}()
	}
}

// testRacyWrite tests that it is safe to mutate the input Write buffer
// immediately after cancelation has occurred.
func testRacyWrite(t *testing.T, c net.Conn) {
	go chunkedCopy(ioutil.Discard, c)

	var wg sync.WaitGroup
	defer wg.Wait()

	c.SetWriteDeadline(time.Now().Add(time.Millisecond))
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			b1 := make([]byte, 1024)
			b2 := make([]byte, 1024)
			for j := 0; j < 100; j++ {
				_, err := c.Write(b1)
				copy(b1, b2) // Mutate b1 to trigger potential race
				if err != nil {
					checkForTimeoutError(t, err)
					c.SetWriteDeadline(time.Now().Add(time.Millisecond))
				}
			}
		}()
	}
}

// testReadTimeout tests that Read timeouts do not affect Write.
func testReadTimeout(t *testing.T, c net.Conn) {
	go chunkedCopy(ioutil.Discard, c)

	c.SetReadDeadline(aLongTimeAgo)
	_, err := c.Read(make([]byte, 1024))
	checkForTimeoutError(t, err)
	if _, err := c.Write(make([]byte, 1024)); err != nil {
		t.Errorf("unexpected Write error: %v", err)
	}
}

// testWriteTimeout tests that Write timeouts do not affect Read.
func testWriteTimeout(t *testing.T, c net.Conn) {
	go chunkedCopy(c, rand.New(rand.NewSource(0)))

	c.SetWriteDeadline(aLongTimeAgo)
	_, err := c.Write(make([]byte, 1024))
	checkForTimeoutError(t, err)
	if _, err := c.Read(make([]byte, 1024)); err != nil {
		t.Errorf("unexpected Read error: %v", err)
	}
}

// testPastTimeout tests that a deadline set in the past immediately times out
// Read and Write requests.
func testPastTimeout(t *testing.T, c net.Conn) {
	go chunkedCopy(c, c)

	testRoundtrip(t, c)

	c.SetDeadline(aLongTimeAgo)
	n, err := c.Write(make([]byte, 1024))
	if n != 0 {
		t.Errorf("unexpected Write count: got %d, want 0", n)
	}
	checkForTimeoutError(t, err)
	n, err = c.Read(make([]byte, 1024))
	if n != 0 {
		t.Errorf("unexpected Read count: got %d, want 0", n)
	}
	checkForTimeoutError(t, err)

	testRoundtrip(t, c)
}

// testPresentTimeout tests that a past deadline set while there are pending
// Read and Write operations immediately times out those operations.
func testPresentTimeout(t *testing.T, c net.Conn) {
	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Add(3)

	deadlineSet := make(chan bool, 1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		deadlineSet <- true
		c.SetReadDeadline(aLongTimeAgo)
		c.SetWriteDeadline(aLongTimeAgo)
	}()
	go func() {
		defer wg.Done()
		n, err := c.Read(make([]byte, 1024))
		if n != 0 {
			t.Errorf("unexpected Read count: got %d, want 0", n)
		}
		checkForTimeoutError(t, err)
		if len(deadlineSet) == 0 {
			t.Error("Read timed out before deadline is set")
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		for err == nil {
			_, err = c.Write(make([]byte, 1024))
		}
		checkForTimeoutError(t, err)
		if len(deadlineSet) == 0 {
			t.Error("Write timed out before deadline is set")
		}
	}()
}

// testFutureTimeout tests that a future deadline will eventually time out
// Read and Write operations.
func testFutureTimeout(t *testing.T, c net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	c.SetDeadline(time.Now().Add(100 * time.Millisecond))
	go func() {
		defer wg.Done()
		_, err := c.Read(make([]byte, 1024))
		checkForTimeoutError(t, err)
	}()
	go func() {
		defer wg.Done()
		var err error
		for err == nil {
			_, err = c.Write(make([]byte, 1024))
		}
		checkForTimeoutError(t, err)
	}()
	wg.Wait()

	go chunkedCopy(c, c)
	resyncConn(t, c)
	testRoundtrip(t, c)
}

// testCloseTimeout tests that calling Close immediately times out pending
// Read and Write operations.
func testCloseTimeout(t *testing.T, c net.Conn) {
	go chunkedCopy(c, c)

	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Add(3)

	// Test for cancelation upon connection closure.
	c.SetDeadline(neverTimeout)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		c.Close()
	}()
	go func() {
		defer wg.Done()
		var err error
		buf := make([]byte, 1024)
		for err == nil {
			_, err = c.Read(buf)
		}
	}()
	go func() {
		defer wg.Done()
		var err error
		buf := make([]byte, 1024)
		for err == nil {
			_, err = c.Write(buf)
		}
	}()
}

// testConcurrentMethods tests that the methods of net.Conn can safely
// be called concurrently.
func testConcurrentMethods(t *testing.T, c net.Conn) {
	if runtime.GOOS == "plan9" {
		t.Skip("skipping on plan9; see https://golang.org/issue/20489")
	}
	go chunkedCopy(c, c)

	// The results of the calls may be nonsensical, but this should
	// not trigger a race detector warning.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(7)
		go func() {
			defer wg.Done()
			c.Read(make([]byte, 1024))
		}()
		go func() {
			defer wg.Done()
			c.Write(make([]byte, 1024))
		}()
		go func() {
			defer wg.Done()
			c.SetDeadline(time.Now().Add(10 * time.Millisecond))
		}()
		go func() {
			defer wg.Done()
			c.SetReadDeadline(aLongTimeAgo)
		}()
		go func() {
			defer wg.Done()
			c.SetWriteDeadline(aLongTimeAgo)
		}()
		go func() {
			defer wg.Done()
			c.LocalAddr()
		}()
		go func() {
			defer wg.Done()
			c.RemoteAddr()
		}()
	}
	wg.Wait() // At worst, the deadline is set 10ms into the future

	resyncConn(t, c)
	testRoundtrip(t, c)
}

// checkForTimeoutError checks that the error satisfies the Error interface
// and that Timeout returns true.
func checkForTimeoutError(t *testing.T, err error) {
	t.Helper()
	if nerr, ok := err.(net.Error); ok {
		if !nerr.Timeout() {
			t.Errorf("err.Timeout() = false, want true")
		}
	} else {
		t.Errorf("got %T, want net.Error", err)
	}
}

// testRoundtrip writes something into c and reads it back.
// It assumes that everything written into c is echoed back to itself.
func testRoundtrip(t *testing.T, c net.Conn) {
	t.Helper()
	if err := c.SetDeadline(neverTimeout); err != nil {
		t.Errorf("roundtrip SetDeadline error: %v", err)
	}

	const s = "Hello, world!"
	buf := []byte(s)
	if _, err := c.Write(buf); err != nil {
		t.Errorf("roundtrip Write error: %v", err)
	}
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Errorf("roundtrip Read error: %v", err)
	}
	if string(buf) != s {
		t.Errorf("roundtrip data mismatch: got %q, want %q", buf, s)
	}
}

// resyncConn resynchronizes the connection into a sane state.
// It assumes that everything written into c is echoed back to itself.
// It assumes that 0xff is not currently on the wire or in the read buffer.
func resyncConn(t *testing.T, c net.Conn) {
	t.Helper()
	c.SetDeadline(neverTimeout)
	errCh := make(chan error)
	go func() {
		_, err := c.Write([]byte{0xff})
		errCh <- err
	}()
	buf := make([]byte, 1024)
	for {
		n, err := c.Read(buf)
		if n > 0 && bytes.IndexByte(buf[:n], 0xff) == n-1 {
			break
		}
		if err != nil {
			t.Errorf("unexpected Read error: %v", err)
			break
		}
	}
	if err := <-errCh; err != nil {
		t.Errorf("unexpected Write error: %v", err)
	}
}

// chunkedCopy copies from r to w in fixed-width chunks to avoid
// causing a Write that exceeds the maximum packet size for packet-based
// connections like "unixpacket".
// We assume that the maximum packet size is at least 1024.
func chunkedCopy(w io.Writer, r io.Reader) error {
	b := make([]byte, 1024)
	_, err := io.CopyBuffer(struct{ io.Writer }{w}, struct{ io.Reader }{r}, b)
	return err
}
