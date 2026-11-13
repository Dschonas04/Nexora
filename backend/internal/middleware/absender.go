package middleware

import (
	"net"
	"net/http"
	"strings"
)

// EchteIP sets r.RemoteAddr to the address of the actual visitor.
//
// The request passes several proxies in the LAN before it gets here (reverse
// proxy, Caddy on the host, the nginx in the frontend container), and each one
// appends the address it received the request from to X-Forwarded-For. The
// list is therefore read from the right: as long as an address belongs to a
// private network it is one of the own proxies, the first public address is
// the visitor.
//
// Read from the left, as chi's RealIP and most examples do, the first entry is
// whatever the visitor wrote into the header himself -- and so is
// True-Client-IP, which RealIP even prefers. Both are ignored here.
//
// Headers are only believed at all if the direct peer is itself in a private
// network; a request that reaches the service directly from outside keeps its
// own address. If every address in the chain is private, the visitor sits in
// the LAN, and the leftmost one is taken.
func EchteIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := echteIP(r.RemoteAddr, r.Header.Values("X-Forwarded-For")); ip != "" {
			r.RemoteAddr = ip
		}
		next.ServeHTTP(w, r)
	})
}

func echteIP(remote string, xff []string) string {
	host := remote
	if h, _, err := net.SplitHostPort(remote); err == nil {
		host = h
	}
	direkt := net.ParseIP(host)
	if direkt == nil || !intern(direkt) {
		return ""
	}
	var kette []net.IP
	for _, zeile := range xff {
		for _, teil := range strings.Split(zeile, ",") {
			teil = strings.TrimSpace(teil)
			if h, _, err := net.SplitHostPort(teil); err == nil {
				teil = h
			}
			ip := net.ParseIP(strings.Trim(teil, "[]"))
			if ip == nil {
				// Garbage in the header: nothing to the left of it can be trusted.
				kette = kette[:0]
				continue
			}
			kette = append(kette, ip)
		}
	}
	for i := len(kette) - 1; i >= 0; i-- {
		if !intern(kette[i]) {
			return kette[i].String()
		}
	}
	if len(kette) > 0 {
		return kette[0].String()
	}
	return direkt.String()
}

func intern(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
