package main

import "testing"

func TestColorFamily(t *testing.T) {
	cases := map[string]string{
		"#f5f0e1": "white", // cream
		"#f0e8d8": "white", // warm off-white
		"#ffffff": "white",
		"#808080": "gray",
		"#101010": "black",
		"#e5484d": "red",
		"#f76b15": "orange",
		"#f5c443": "yellow",
		"#46a758": "green",
		"#3e7bfa": "blue",
		"#8e4ec6": "purple",
		"#e93d82": "pink",
		"#8f6a4a": "brown",
		"#a0d050": "green",
	}
	for hex, want := range cases {
		if got := colorFamily(hex); got != want {
			t.Errorf("colorFamily(%s) = %s, want %s", hex, got, want)
		}
	}
}
