// Package health provides dependency-free reachability and latency probes used
// by the engine to decide which connections are up, to rank locations, and to
// drive failover.
//
// Three probe styles are offered:
//
//   - TCPPing:    raw TCP connect latency to a host:port (quick liveness).
//   - DirectHTTP: HTTP(S) GET over the default/WAN path (is the internet up?).
//   - SOCKSHTTP:  HTTP(S) GET through a local SOCKS5 proxy (does the *tunnel*
//     actually carry traffic end-to-end?).
//
// A minimal no-auth SOCKS5 client is implemented inline so the whole binary
// stays free of third-party network dependencies.
package health

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Probe is the outcome of a single reachability check.
type Probe struct {
	OK        bool
	LatencyMs int
	Err       error
}

// DefaultTimeout is used when a caller passes a non-positive timeout.
const DefaultTimeout = 6 * time.Second

func norm(t time.Duration) time.Duration {
	if t <= 0 {
		return DefaultTimeout
	}
	return t
}

// TCPPing dials host:port over TCP and reports the connect latency. This is the
// cheapest liveness signal and is used to rank subscription locations.
func TCPPing(ctx context.Context, host string, port int, timeout time.Duration) Probe {
	timeout = norm(timeout)
	start := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return Probe{Err: err}
	}
	_ = conn.Close()
	return Probe{OK: true, LatencyMs: int(time.Since(start).Milliseconds())}
}

// DirectHTTP probes url over the default network path (used to check the WAN).
func DirectHTTP(ctx context.Context, url string, timeout time.Duration) Probe {
	timeout = norm(timeout)
	d := &net.Dialer{Timeout: timeout}
	return httpProbe(ctx, url, d.DialContext, timeout)
}

// SOCKSHTTP probes url through a local SOCKS5 proxy at socksAddr (host:port).
// A 2xx/3xx response means the tunnel behind the proxy is genuinely working.
func SOCKSHTTP(ctx context.Context, socksAddr, url string, timeout time.Duration) Probe {
	timeout = norm(timeout)
	dial := func(ctx context.Context, _ /*network*/, addr string) (net.Conn, error) {
		return socks5Dial(ctx, socksAddr, addr, timeout)
	}
	return httpProbe(ctx, url, dial, timeout)
}

func httpProbe(
	ctx context.Context,
	url string,
	dial func(ctx context.Context, network, addr string) (net.Conn, error),
	timeout time.Duration,
) Probe {
	tr := &http.Transport{
		DialContext:           dial,
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		ForceAttemptHTTP2:     false,
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: tr,
		// Don't follow redirects; a 3xx from a generate_204-style endpoint is
		// still proof the path works.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Probe{Err: err}
	}
	req.Header.Set("User-Agent", "keen-manager/health")
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return Probe{Err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return Probe{OK: true, LatencyMs: int(time.Since(start).Milliseconds())}
	}
	return Probe{Err: fmt.Errorf("unexpected status %d", resp.StatusCode)}
}

// socks5Dial performs a no-auth SOCKS5 CONNECT to targetAddr via proxyAddr and
// returns the established connection (ready for the caller to run TLS/HTTP on).
func socks5Dial(ctx context.Context, proxyAddr, targetAddr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("dial socks proxy: %w", err)
	}
	// Bound the handshake by the context deadline or the timeout.
	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	fail := func(e error) (net.Conn, error) {
		_ = conn.Close()
		return nil, e
	}

	// Greeting: version 5, 1 method, no-auth (0x00).
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return fail(err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fail(err)
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		return fail(errors.New("socks5: server rejected no-auth"))
	}

	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return fail(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fail(fmt.Errorf("socks5: bad port %q", portStr))
	}

	// CONNECT request.
	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			req = append(req, 0x01)
			req = append(req, v4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return fail(errors.New("socks5: hostname too long"))
		}
		req = append(req, 0x03, byte(len(host)))
		req = append(req, host...)
	}
	var pb [2]byte
	binary.BigEndian.PutUint16(pb[:], uint16(port))
	req = append(req, pb[:]...)
	if _, err := conn.Write(req); err != nil {
		return fail(err)
	}

	// Reply header: VER REP RSV ATYP.
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fail(err)
	}
	if head[1] != 0x00 {
		return fail(fmt.Errorf("socks5: connect failed (reply code %d)", head[1]))
	}
	// Drain the bound address so the stream is positioned at payload start.
	switch head[3] {
	case 0x01: // IPv4 + port
		_, _ = io.CopyN(io.Discard, conn, net.IPv4len+2)
	case 0x04: // IPv6 + port
		_, _ = io.CopyN(io.Discard, conn, net.IPv6len+2)
	case 0x03: // domain
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return fail(err)
		}
		_, _ = io.CopyN(io.Discard, conn, int64(l[0])+2)
	default:
		return fail(fmt.Errorf("socks5: unknown address type %d", head[3]))
	}

	_ = conn.SetDeadline(time.Time{}) // hand back a clean connection
	return conn, nil
}
