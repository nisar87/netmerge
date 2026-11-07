package main

import (
	"math"
	"sync"
)

// Quality of a line from its recent health probes: how many were lost, how much the response time jumps about
// (jitter) and how slow it is. Shown as Excellent / Good / Fair / Poor.

type qSample struct {
	ok bool
	ms int
}

const qWindow = 30 // probes (about 5 minutes)

var (
	qualMu sync.Mutex
	qual   = map[string][]qSample{}
)

func qualityAdd(name string, ok bool, ms int) {
	qualMu.Lock()
	defer qualMu.Unlock()
	q := append(qual[name], qSample{ok, ms})
	if len(q) > qWindow {
		q = q[len(q)-qWindow:]
	}
	qual[name] = q
}

func qualityReset(name string) {
	qualMu.Lock()
	delete(qual, name)
	qualMu.Unlock()
}

type Quality struct {
	Score  int     `json:"score"` // 0-100, 0 with Label "" = not enough data yet
	Label  string  `json:"label"`
	Loss   float64 `json:"loss"`   // percent of probes lost
	Jitter int     `json:"jitter"` // ms
	Avg    int     `json:"avg"`    // ms
}

func qualityOf(samples []qSample) Quality {
	if len(samples) < 5 {
		return Quality{}
	}
	lost, n := 0, 0
	var sum, jit float64
	prev := -1
	jn := 0
	for _, s := range samples {
		if !s.ok {
			lost++
			continue
		}
		n++
		sum += float64(s.ms)
		if prev >= 0 {
			jit += math.Abs(float64(s.ms - prev))
			jn++
		}
		prev = s.ms
	}
	q := Quality{Loss: float64(lost) * 100 / float64(len(samples))}
	if n > 0 {
		q.Avg = int(sum / float64(n))
	}
	if jn > 0 {
		q.Jitter = int(jit / float64(jn))
	}
	score := 100.0
	score -= math.Min(60, q.Loss*6)
	score -= math.Min(25, float64(q.Jitter)/4)
	if q.Avg > 80 {
		score -= math.Min(20, float64(q.Avg-80)/15)
	}
	if score < 0 {
		score = 0
	}
	q.Score = int(score)
	switch {
	case q.Score >= 85:
		q.Label = "Excellent"
	case q.Score >= 65:
		q.Label = "Good"
	case q.Score >= 40:
		q.Label = "Fair"
	default:
		q.Label = "Poor"
	}
	return q
}

func qualityFor(name string) Quality {
	qualMu.Lock()
	s := append([]qSample(nil), qual[name]...)
	qualMu.Unlock()
	return qualityOf(s)
}
