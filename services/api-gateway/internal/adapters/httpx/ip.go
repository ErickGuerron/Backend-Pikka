package httpx

import (
	"net"
	"net/http"
	"strings"
)

// remoteHost returns the host part of the peer address, without port.
func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ExtractIPDirect uses the TCP peer address and ignores every forwarding header.
func ExtractIPDirect() func(*http.Request) string {
	return remoteHost
}

// ExtractIPFromXFFHeader walks X-Forwarded-For from the right and returns the first address that is
// not a trusted proxy (loopback, link-local or private). Unparseable entries make it fall back to the
// peer address, so a malformed header cannot choose the client IP.
func ExtractIPFromXFFHeader() func(*http.Request) string {
	return func(r *http.Request) string {
		direct := remoteHost(r)
		xffs := r.Header[HeaderXForwardedFor]
		if len(xffs) == 0 {
			return direct
		}
		ips := append(strings.Split(strings.Join(xffs, ","), ","), direct)
		for i := len(ips) - 1; i >= 0; i-- {
			ips[i] = strings.TrimSpace(ips[i])
			ips[i] = strings.TrimPrefix(ips[i], "[")
			ips[i] = strings.TrimSuffix(ips[i], "]")
			ip := net.ParseIP(ips[i])
			if ip == nil {
				return direct
			}
			if !isTrustedProxy(ip) {
				return ip.String()
			}
		}
		return strings.TrimSpace(ips[0])
	}
}

func isTrustedProxy(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate()
}

// schemeOf reports the scheme of the client-facing connection. Forwarding headers count only when the
// direct peer is a trusted proxy, the same rule echo applies.
func schemeOf(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if !isTrustedProxy(net.ParseIP(remoteHost(r))) {
		return "http"
	}
	if p := r.Header.Get(HeaderXForwardedProto); p != "" {
		return strings.ToLower(p)
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Ssl"), "on") {
		return "https"
	}
	if s := r.Header.Get("X-Url-Scheme"); s != "" {
		return strings.ToLower(s)
	}
	return "http"
}
