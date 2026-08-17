package main

import (
	"os"
	"testing"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

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

func TestLoadInjection(t *testing.T) {
	file := t.TempDir() + "/snippet.html"
	if err := writeFile(file, `<script src="/x.js"></script>`); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		file   string
		inline string
		want   string
	}{
		{"neither", "", "", ""},
		{"inline only", "", "<p>hi</p>", "<p>hi</p>"},
		{"file wins", file, "<p>inline</p>", `<script src="/x.js"></script>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := loadInjection(tc.file, tc.inline)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("loadInjection = %q, want %q", got, tc.want)
			}
		})
	}
	if _, err := loadInjection(file+".nope", ""); err == nil {
		t.Error("missing inject file should error")
	}
}
