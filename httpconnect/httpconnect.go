// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package httpconnect provides an implementation of a Dialer that connects
// to the destination address via a HTTP(S) proxy.

// ALL CREDITS TO https://go-review.googlesource.com/c/net/+/134675
package httpconnect

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"
)

// deadline struct is copied from net/pipe.go
type deadline struct {
	mu     sync.Mutex // Guards timer and cancel
	timer  *time.Timer
	cancel chan struct{} // Must be non-nil
}

func (d *deadline) set(t time.Time) {
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

func (d *deadline) wait() chan struct{} {
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

type ioResult struct {
	b   []byte
	n   int
	err error
}

type connError struct {
	errStr  string
	timeout bool
}

func (ce connError) Error() string   { return ce.errStr }
func (ce connError) Timeout() bool   { return ce.timeout }
func (ce connError) Temporary() bool { return ce.timeout }

var (
	errDeadline = connError{"deadline exceeded", true}
	errClosed   = connError{"closed connection", false}
)

func newDialerConn() *dialerConn {
	return &dialerConn{
		done:         make(chan struct{}),
		readDeadline: deadline{cancel: make(chan struct{})},
	}
}

type dialerConn struct {
	w              net.Conn
	r              io.ReadCloser
	localAddr      net.Addr
	remoteAddr     net.Addr
	readDeadline   deadline
	readMu         sync.Mutex
	once           sync.Once // protects closing done
	done           chan struct{}
	storedRead     *ioResult
	readInProgress bool
}

func (c *dialerConn) Write(b []byte) (n int, err error) {
	n, err = c.w.Write(b)
	return
}

func (c *dialerConn) Read(b []byte) (n int, err error) {
	switch {
	case isClosedChan(c.done):
		return 0, errClosed
	case isClosedChan(c.readDeadline.wait()):
		return 0, errDeadline
	}

	// Ensure there aren't multiple reads depending on a previous read that
	// hasn't yet returned
	c.readMu.Lock()
	defer c.readMu.Unlock()
	ioCh := make(chan *ioResult)
	go func(ch chan *ioResult) {
		if c.readInProgress {
			for {
				if c.storedRead != nil {
					ch <- c.storedRead
					return
				}
			}
		} else {
			c.readInProgress = true
			n, err := c.r.Read(b)
			if n == 0 {
				err = io.EOF
			}
			c.storedRead = &ioResult{b[:n], n, err}
			ch <- c.storedRead
		}
	}(ioCh)

	select {
	case <-c.done:
		return 0, errClosed
	case <-c.readDeadline.wait():
		return 0, errDeadline
	case read := <-ioCh:
		// clear the stored read
		c.storedRead = nil
		c.readInProgress = false
		copy(b[:read.n], read.b[:read.n])
		return read.n, read.err
	}
}

func (c *dialerConn) Close() error {
	c.w.Close()                         // close writer
	c.once.Do(func() { close(c.done) }) // close reader
	return nil
}

func (c *dialerConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *dialerConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *dialerConn) SetDeadline(t time.Time) error {
	c.w.SetWriteDeadline(t) // set write deadline
	c.readDeadline.set(t)   // set read deadline
	return nil
}

func (c *dialerConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.set(t)
	return nil
}

func (c *dialerConn) SetWriteDeadline(t time.Time) error {
	c.w.SetWriteDeadline(t)
	return nil
}

func (c *dialerConn) addrTrackingGotConn() func(connInfo httptrace.GotConnInfo) {
	return func(connInfo httptrace.GotConnInfo) {
		c.localAddr = connInfo.Conn.LocalAddr()
		c.remoteAddr = connInfo.Conn.RemoteAddr()
	}
}

// A Dialer holds HTTP CONNECT-specific options.
type Dialer struct {
	proxyNetwork   string   // network between a proxy server and a client
	proxyUrl       *url.URL // proxy server url
	proxyTransport *http.Transport
}

// DialContext connects to the provided address on the provided network.
//
// See func Dial of the net package of standard library for a
// description of the network and address parameters.
// For TCP and UDP networks, the address has the form "host:port"
func (d *Dialer) DialContext(ctx context.Context, network, address string) (conn *dialerConn, err error) {
	switch network {
	case "tcp", "tcp6", "tcp4":
	default:
		return nil, errors.New("network not implemented")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	conn = newDialerConn()
	pr, pw := net.Pipe()

	if d.proxyTransport.ProxyConnectHeader == nil {
		d.proxyTransport.ProxyConnectHeader = make(http.Header)
	}

	connectReq := &http.Request{
		Method: "CONNECT",
		URL:    d.proxyUrl,
		Host:   address,
		Header: d.proxyTransport.ProxyConnectHeader,
		Body:   pr,
	}
	trace := &httptrace.ClientTrace{
		GotConn: conn.addrTrackingGotConn(),
	}
	connectReq = connectReq.WithContext(httptrace.WithClientTrace(ctx, trace))
	resp, err := d.proxyTransport.RoundTrip(connectReq)
	if err != nil {
		return
	}
	if resp.StatusCode != 200 {
		f := strings.SplitN(resp.Status, " ", 2)
		if len(f) < 2 {
			err = errors.New("unknown status code")
			return
		}
		err = errors.New(f[1])
		return
	}
	conn.w = pw
	conn.r = resp.Body
	return
}

// Dial connects to the provided address on the provided network.
//
// Deprecated: Use DialContext instead.
func (d *Dialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

// NewDialer returns a new Dialer that dials through the proxy server's
// provided url. The provided transport will be used for communication between
// the client and proxy.
func NewDialer(network string, url *url.URL, transport *http.Transport) *Dialer {
	if url.Scheme != "http" && url.Scheme != "https" {
		return nil
	}
	// Copy the credentials for the proxy to the Transport
	if url.User != nil {
		if transport.ProxyConnectHeader == nil {
			transport.ProxyConnectHeader = make(http.Header)
		}
		password, _ := url.User.Password()
		encodedAuth := base64.StdEncoding.EncodeToString([]byte(url.User.Username() + ":" + password))
		transport.ProxyConnectHeader.Set("Proxy-Authorization", "Basic "+encodedAuth)
	}
	return &Dialer{
		proxyNetwork:   network,
		proxyUrl:       url,
		proxyTransport: transport,
	}
}
