package main

import "testing"

func TestListenAddr(t *testing.T) {
	cases := []struct {
		port string
		want string
	}{
		{"", ""},
		{"8080", ":8080"},
		{":8080", ":8080"},
		{"127.0.0.1:9000", "127.0.0.1:9000"},
		{"abc", ""},
	}
	for _, tc := range cases {
		if got := listenAddr(tc.port); got != tc.want {
			t.Errorf("listenAddr(%q) = %q, want %q", tc.port, got, tc.want)
		}
	}
}
