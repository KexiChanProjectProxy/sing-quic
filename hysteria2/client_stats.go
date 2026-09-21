package hysteria2

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/sagernet/quic-go/http3"
	"github.com/sagernet/sing-quic/hysteria2/internal/protocol"
)

// statsResponseLimit bounds how much of a statistics response is read. A real
// response is well under a kilobyte, padding header aside.
const statsResponseLimit = 4096

// defaultStatsTimeout bounds a statistics request when the caller gives no
// deadline of its own.
const defaultStatsTimeout = 5 * time.Second

// RequestServerStats makes connections opened from now on ask the server to
// report its view of the connection. It does not affect a connection that is
// already open, and it is harmless against a server that does not support
// reporting: the request is ignored and ServerStats keeps returning false.
func (c *Client) RequestServerStats() {
	c.statsRequested.Store(true)
}

// activeConnection returns the current connection without opening one.
// Statistics describe traffic that already exists, so asking for them must
// never be the reason a connection is dialed.
func (c *Client) activeConnection() *clientQUICConnection {
	c.connAccess.Lock()
	defer c.connAccess.Unlock()
	conn := c.conn
	if conn == nil || !conn.active() {
		return nil
	}
	return conn
}

// LocalStats returns this client's view of its current connection, as the
// sender of upstream traffic. It returns false when no connection is open.
func (c *Client) LocalStats() (TransportStats, bool) {
	conn := c.activeConnection()
	if conn == nil {
		return TransportStats{}, false
	}
	return connectionStats(conn.id, conn.quicConn, conn.losses), true
}

// ServerStats asks the server for its view of the current connection, as the
// sender of downstream traffic. It returns false when no connection is open,
// when the server did not agree to report statistics on it, or when the
// request fails.
//
// A failure never tears down or replaces the connection: the request only ever
// uses the connection that is already open. After a request fails for any
// reason but its own deadline, the HTTP/3 transport drops its handle on the
// connection and later requests fail until the next connection. The handle
// cannot be rebuilt in place, because HTTP/3 allows a single control stream per
// connection.
func (c *Client) ServerStats(ctx context.Context) (TransportStats, bool) {
	conn := c.activeConnection()
	if conn == nil || !conn.serverStats {
		return TransportStats{}, false
	}
	transport, canRestrict := conn.http3Transport.(*http3.Transport)
	if !canRestrict {
		// Without OnlyCachedConn a failed request could make the transport dial
		// a new QUIC connection over the socket this one is using.
		return TransportStats{}, false
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultStatsTimeout)
		defer cancel()
	}
	request := (&http.Request{
		Method: http.MethodGet,
		URL: &url.URL{
			Scheme: "https",
			Host:   protocol.URLHost,
			Path:   protocol.URLPathStats,
		},
		Header: make(http.Header),
	}).WithContext(ctx)
	protocol.SetStatsPadding(request.Header)
	response, err := transport.RoundTripOpt(request, http3.RoundTripOpt{OnlyCachedConn: true})
	if err != nil {
		return TransportStats{}, false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return TransportStats{}, false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, statsResponseLimit))
	if err != nil {
		return TransportStats{}, false
	}
	stats, ok := decodeTransportStats(string(body))
	if !ok {
		return TransportStats{}, false
	}
	stats.ConnectionID = conn.id
	return stats, true
}
