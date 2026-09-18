package poolsync

import (
	"net"
	"os"
	"strings"

	"hackme/internal/netutil"
)

// ResolveOrdersSettleBase returns the node URL coordinators should relay fuzz escrow
// settlements to, and whether pull-mode is required (loopback / unreachable from VPS).
func ResolveOrdersSettleBase() (base string, pull bool) {
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv("HACKME_ORDERS_SETTLE_BASE")), "/"); v != "" {
		return v, isLoopbackSettleBase(v)
	}
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv("BASE_URL")), "/"); v != "" {
		if !isLoopbackSettleBase(v) {
			return v, false
		}
	}
	bind := strings.TrimSpace(os.Getenv("HACKME_BIND_ADDR"))
	if bind == "" {
		bind = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		host = bind
	}
	if isLoopbackHost(host) {
		return "http://" + bind, true
	}
	if !strings.Contains(bind, "://") {
		return "http://" + bind, false
	}
	return strings.TrimRight(bind, "/"), false
}

func isLoopbackSettleBase(u string) bool {
	return netutil.IsLoopbackURL(u)
}

func isLoopbackHost(host string) bool {
	return netutil.IsLoopbackHost(host)
}
