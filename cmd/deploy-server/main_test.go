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

func TestDefaultHTTPAddr(t *testing.T) {
	cases := []struct {
		port string
		want string
	}{
		{"", ":8080"},
		{"1234", ":1234"},
		{":9999", ":9999"},
		{"127.0.0.1:7000", "127.0.0.1:7000"},
		{"garbage", ":8080"},
	}
	for _, tc := range cases {
		t.Setenv("PORT", tc.port)
		if got := defaultHTTPAddr(); got != tc.want {
			t.Errorf("defaultHTTPAddr(PORT=%q) = %q, want %q", tc.port, got, tc.want)
		}
	}
}
