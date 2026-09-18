package hms

import (
	"net/http"
	"testing"
)

func TestLoopbackOnlyRejectsEmptyRemoteAddr(t *testing.T) {
	r := &http.Request{RemoteAddr: ""}
	if loopbackOnly(r) {
		t.Fatal("empty RemoteAddr must not count as loopback")
	}
	r.RemoteAddr = "127.0.0.1:9999"
	if !loopbackOnly(r) {
		t.Fatal("127.0.0.1 must be loopback")
	}
	r.RemoteAddr = "10.0.0.5:443"
	if loopbackOnly(r) {
		t.Fatal("RFC1918 must not be loopback")
	}
}
