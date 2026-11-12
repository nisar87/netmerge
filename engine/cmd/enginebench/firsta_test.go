package main

import "testing"

func TestFirstA(t *testing.T) {
	q := buildQuery("example.com")
	r := append([]byte{}, q...)
	r[2], r[3], r[7] = 0x81, 0x80, 2 // two answers: a CNAME, then the A record
	r = append(r, 0xc0, 12, 0, 5, 0, 1, 0, 0, 0, 60, 0, 2, 0xc0, 12)
	r = append(r, 0xc0, 12, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 198, 18, 0, 7)
	if a := firstA(r); a.String() != "198.18.0.7" {
		t.Fatalf("got %v", a)
	}
}
