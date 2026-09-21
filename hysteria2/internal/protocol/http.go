package protocol

import (
	"net/http"
	"strconv"
)

const (
	URLHost      = "hysteria"
	URLPath      = "/auth"
	URLPathStats = "/stats"

	RequestHeaderAuth        = "Hysteria-Auth"
	ResponseHeaderUDPEnabled = "Hysteria-UDP"
	CommonHeaderCCRX         = "Hysteria-CC-RX"
	CommonHeaderPadding      = "Hysteria-Padding"

	// CommonHeaderStats negotiates transport statistics reporting. A client
	// that wants the server's view of the connection sends it on the auth
	// request, and a server that can report sends it back on the auth response.
	// Implementations that do not know it ignore it on both sides, and a client
	// never asks for statistics until the server has answered with it.
	CommonHeaderStats = "Hysteria-Stats"

	StatusAuthOK = 233
)

// AuthRequest is what client sends to server for authentication.
type AuthRequest struct {
	Auth string
	Rx   uint64 // 0 = unknown, client asks server to use bandwidth detection
}

// AuthResponse is what server sends to client when authentication is passed.
type AuthResponse struct {
	UDPEnabled bool
	Rx         uint64 // 0 = unlimited
	RxAuto     bool   // true = server asks client to use bandwidth detection
}

func AuthRequestFromHeader(h http.Header) AuthRequest {
	rx, _ := strconv.ParseUint(h.Get(CommonHeaderCCRX), 10, 64)
	return AuthRequest{
		Auth: h.Get(RequestHeaderAuth),
		Rx:   rx,
	}
}

func AuthRequestToHeader(h http.Header, req AuthRequest) {
	h.Set(RequestHeaderAuth, req.Auth)
	h.Set(CommonHeaderCCRX, strconv.FormatUint(req.Rx, 10))
	h.Set(CommonHeaderPadding, authRequestPadding.String())
}

func AuthResponseFromHeader(h http.Header) AuthResponse {
	resp := AuthResponse{}
	resp.UDPEnabled, _ = strconv.ParseBool(h.Get(ResponseHeaderUDPEnabled))
	rxStr := h.Get(CommonHeaderCCRX)
	if rxStr == "auto" {
		// Special case for server requesting client to use bandwidth detection
		resp.RxAuto = true
	} else {
		resp.Rx, _ = strconv.ParseUint(rxStr, 10, 64)
	}
	return resp
}

func AuthResponseToHeader(h http.Header, resp AuthResponse) {
	h.Set(ResponseHeaderUDPEnabled, strconv.FormatBool(resp.UDPEnabled))
	if resp.RxAuto {
		h.Set(CommonHeaderCCRX, "auto")
	} else {
		h.Set(CommonHeaderCCRX, strconv.FormatUint(resp.Rx, 10))
	}
	h.Set(CommonHeaderPadding, authResponsePadding.String())
}

// StatsRequested reports whether an auth request asked for statistics.
func StatsRequested(h http.Header) bool {
	return h.Get(CommonHeaderStats) == "1"
}

// SetStatsRequested marks an auth request or response as supporting
// statistics.
func SetStatsRequested(h http.Header) {
	h.Set(CommonHeaderStats, "1")
}

// SetStatsPadding pads a statistics request or response.
func SetStatsPadding(h http.Header) {
	h.Set(CommonHeaderPadding, statsPadding.String())
}
