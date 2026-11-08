package main

import "testing"

func TestQuoteArgsKeepsFoldersWithSpacesTogether(t *testing.T) {
	got := quoteArgs([]string{"-background", "-root", `C:\Users\Nisar Ahmad\My Docs\nm test`, "-x"})
	want := `-background -root "C:\Users\Nisar Ahmad\My Docs\nm test" -x`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestQuoteArgEdgeCases(t *testing.T) {
	for in, want := range map[string]string{"": `""`, "plain": "plain", `a"b`: `"a\"b"`, `C:\x y\`: `"C:\x y\\"`} {
		if got := quoteArg(in); got != want {
			t.Errorf("%q: got %s want %s", in, got, want)
		}
	}
}
