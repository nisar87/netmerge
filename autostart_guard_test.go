package main

import "testing"

func TestAutoStartRefusedForACopyWithItsOwnRoot(t *testing.T) {
	if err := autoStartAllowed(`C:\Apps\NetMerge\NetMerge.exe`, `C:\Apps\NetMerge`); err != nil {
		t.Fatalf("normal install refused: %v", err)
	}
	if err := autoStartAllowed(`C:\Apps\NetMerge\NetMerge.exe`, `c:\apps\netmerge\`); err != nil {
		t.Fatalf("same folder, other spelling, refused: %v", err)
	}
	if err := autoStartAllowed(`C:\nmtest\NetMerge.exe`, `C:\nmtest\data`); err == nil {
		t.Fatal("a copy with its own -root must not take over the start-up task")
	}
}
