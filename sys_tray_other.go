//go:build !windows

package main

func runTray(url string)             {}
func trayRemove()                    {}
func trayBalloon(title, text string) {}
