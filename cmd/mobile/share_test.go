package main

import "testing"

func TestSharedLink(t *testing.T) {
	cases := map[string]string{
		"https://example.test/film/1/":                        "https://example.test/film/1/",
		"Bir Şair - A Poet izle\nhttps://www.example.test/a/": "https://www.example.test/a/",
		"no link here": "",
	}
	for text, want := range cases {
		if got := sharedLink(text); got != want {
			t.Errorf("sharedLink(%q) = %q, want %q", text, got, want)
		}
	}
}
