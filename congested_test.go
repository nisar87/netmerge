package main

import "testing"

// Readings from the real PC's log (rates.log, 12 Nov): speeds in Mbit/s, response times in ms.
func TestCongestedLine(t *testing.T) {
	mb := func(m float64) float64 { return m * 1e6 / 8 }
	cases := []struct {
		name                 string
		rx, capRx, tx, capTx float64
		delay, base          int
		want                 bool
	}{
		// 19:05:35 speed test: the cable carries 7 of its usual 64, its response time went from 90 to 428 ms
		{"cable struggling", 7, 64, 0.2, 98, 428, 90, true},
		// the same moment, Wi-Fi: full but answering normally
		{"Wi-Fi full and fine", 30, 40, 0.7, 47, 87, 85, false},
		// 18:46:08: the cable at 63 of 87 with 1.1 s response time - near its speed, so it is just busy
		{"cable busy at speed", 63, 87, 1.7, 91, 1103, 90, false},
		// 19:05:51 upload at full speed
		{"cable uploading at speed", 2, 64, 98, 98, 453, 90, false},
		// a quiet line whose response time jumps (no traffic of its own): not judged
		{"idle line, slow probe", 0.1, 64, 0.1, 98, 600, 90, false},
		// a line with room and a normal response time
		{"healthy with room", 20, 87, 1, 91, 95, 90, false},
		// response time doubled only
		{"slightly slower", 10, 87, 1, 91, 200, 90, false},
	}
	for _, c := range cases {
		if got := congestedNow(mb(c.rx), mb(c.capRx), mb(c.tx), mb(c.capTx), c.delay, c.base); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	// the usual response time follows lower readings at once and a long run of high ones only slowly
	b := 0
	for _, d := range []int{95, 90, 88} {
		b = trackDelay(b, d)
	}
	for i := 0; i < 10; i++ {
		b = trackDelay(b, 450)
	}
	if b < 88 || b > 200 {
		t.Fatalf("usual response time after 10 slow readings: %d", b)
	}
}
