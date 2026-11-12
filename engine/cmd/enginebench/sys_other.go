//go:build !windows

package main

import (
	"errors"
	"time"
)

func procStats(pid int) (uint64, uint64, time.Duration, error) {
	return 0, 0, 0, errors.New("windows only")
}
func defaultInterface() (string, error) { return "", errors.New("windows only") }
