package main

import "testing"

func TestEnvUint64(t *testing.T) {
	cases := []struct {
		name string
		set  string
		val  string
		want uint64
	}{
		{name: "unset returns fallback", set: "", want: 4194304},
		{name: "plain value", set: "GPU_CHUNK_TEST", val: "8388608", want: 8388608},
		{name: "whitespace trimmed", set: "GPU_CHUNK_TEST", val: " 16777216 ", want: 16777216},
		{name: "zero rejected", set: "GPU_CHUNK_TEST", val: "0", want: 4194304},
		{name: "negative rejected", set: "GPU_CHUNK_TEST", val: "-1", want: 4194304},
		{name: "garbage rejected", set: "GPU_CHUNK_TEST", val: "4M", want: 4194304},
		{name: "overflow rejected", set: "GPU_CHUNK_TEST", val: "99999999999999999999999", want: 4194304},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set != "" {
				t.Setenv(tc.set, tc.val)
			}
			if got := envUint64("GPU_CHUNK_TEST", 4194304); got != tc.want {
				t.Fatalf("envUint64 = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEnvIntMs(t *testing.T) {
	cases := []struct {
		name string
		set  string
		val  string
		want int
	}{
		{name: "unset returns fallback", set: "", want: 2500},
		{name: "plain value", set: "SEARCH_TIMEOUT_TEST", val: "12000", want: 12000},
		{name: "whitespace trimmed", set: "SEARCH_TIMEOUT_TEST", val: " 6000 ", want: 6000},
		{name: "zero allowed (cooldown semantics: no sleep)", set: "SEARCH_TIMEOUT_TEST", val: "0", want: 0},
		{name: "negative rejected", set: "SEARCH_TIMEOUT_TEST", val: "-5", want: 2500},
		{name: "garbage rejected", set: "SEARCH_TIMEOUT_TEST", val: "2.5s", want: 2500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set != "" {
				t.Setenv(tc.set, tc.val)
			}
			if got := envIntMs("SEARCH_TIMEOUT_TEST", 2500); got != tc.want {
				t.Fatalf("envIntMs = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEnvIntPositive(t *testing.T) {
	cases := []struct {
		name string
		set  string
		val  string
		want int
	}{
		{name: "unset returns fallback", set: "", want: 2500},
		{name: "plain value", set: "SEARCH_TIMEOUT_TEST", val: "12000", want: 12000},
		{name: "whitespace trimmed", set: "SEARCH_TIMEOUT_TEST", val: " 6000 ", want: 6000},
		// Regression: a zero GPU search timeout is an already-expired context, so
		// every GPU Search would fail and silently fall back to CPU.
		{name: "zero rejected (expired context guard)", set: "SEARCH_TIMEOUT_TEST", val: "0", want: 2500},
		{name: "negative rejected", set: "SEARCH_TIMEOUT_TEST", val: "-5", want: 2500},
		{name: "garbage rejected", set: "SEARCH_TIMEOUT_TEST", val: "2.5s", want: 2500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set != "" {
				t.Setenv(tc.set, tc.val)
			}
			if got := envIntPositive("SEARCH_TIMEOUT_TEST", 2500); got != tc.want {
				t.Fatalf("envIntPositive = %d, want %d", got, tc.want)
			}
		})
	}
}
