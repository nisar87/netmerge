//go:build !windows

package main

import "fmt"

func winvpnSupported() bool      { return false }
func winvpnInstalledPFN() string { return "" }
func winvpnInstall(string) error { return fmt.Errorf("only on Windows") }
func winvpnRun(string, string) (string, error) {
	return "", fmt.Errorf("only on Windows")
}
