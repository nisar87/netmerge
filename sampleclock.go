package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// stepClock notes where the time between two readings of the byte counters went, so a "sampler was held up" line
// can name the slow step (the adapter list, the engine, a lock held elsewhere) or show that the sampler was not run
// at all for a while (the PC busy or paused). Used only by the stats loop.
type stepClock struct {
	mark  time.Time
	spent map[string]time.Duration
}

var sampleClock stepClock

// step adds the time since the last mark to the named step (the step that has just ended) and moves the mark.
func (c *stepClock) step(name string, now time.Time) {
	if !c.mark.IsZero() && now.After(c.mark) {
		if c.spent == nil {
			c.spent = map[string]time.Duration{}
		}
		c.spent[name] += now.Sub(c.mark)
	}
	c.mark = now
}

// reset starts a new interval at now.
func (c *stepClock) reset(now time.Time) {
	c.spent = nil
	c.mark = now
}

// slowest lists the steps that took at least min, slowest first, for example
// "[reading the adapter list 2.1s, waiting for the next tick 0.9s]".
func (c *stepClock) slowest(min time.Duration) string {
	type st struct {
		name string
		d    time.Duration
	}
	var list []st
	for n, d := range c.spent {
		if d >= min {
			list = append(list, st{n, d})
		}
	}
	if len(list) == 0 {
		return "[no step took long]"
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].d != list[j].d {
			return list[i].d > list[j].d
		}
		return list[i].name < list[j].name
	})
	parts := make([]string, len(list))
	for i, s := range list {
		parts[i] = fmt.Sprintf("%s %v", s.name, s.d.Round(100*time.Millisecond))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
