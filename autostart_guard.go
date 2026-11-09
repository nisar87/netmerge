package main

import (
	"errors"
	"path"
	"strings"
)

// sameFolder compares two Windows folder names without caring about case, slashes or a trailing slash.
func sameFolder(a, b string) bool {
	n := func(p string) string { return strings.ToLower(path.Clean(strings.ReplaceAll(p, "\\", "/"))) }
	return n(a) == n(b)
}

func dirOf(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return "."
}

// autoStartAllowed says whether "start with Windows" may be switched on. The start-up task is one per computer
// (named NetMerge) and starts the program without a folder option, so a copy that was started with -root (a test
// or development copy) must not replace the task of the real installation.
func autoStartAllowed(exe, root string) error {
	if root == "" || sameFolder(root, dirOf(exe)) {
		return nil
	}
	return errors.New("this copy was started with its own data folder (-root), so it will not take over the start-up task; use the normal installation for that")
}
