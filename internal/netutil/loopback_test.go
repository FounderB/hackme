package netutil

import "testing"

func TestIsLoopbackURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"http://127.0.0.1:18081", true},
		{"http://localhost:8080", true},
		{"http://[::1]:8080", true},
		{"https://hackme.tech/pool", false},
		{"http://127.0.0.1.attacker.example/", false},
		{"http://evil.com/path?x=127.0.0.1", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsLoopbackURL(tc.in); got != tc.want {
			t.Fatalf("%q: got %v want %v", tc.in, got, tc.want)
		}
	}
}

func TestLooksRemoteCoordinatorURL(t *testing.T) {
	if LooksRemoteCoordinatorURL("http://127.0.0.1:18081") {
		t.Fatal("loopback should not look remote")
	}
	if !LooksRemoteCoordinatorURL("https://hackme.tech/pool/coordinator") {
		t.Fatal("public URL should look remote")
	}
}
