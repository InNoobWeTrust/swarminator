package tutorial

import (
	"strings"
	"testing"
)

func TestLookupAndAliases(t *testing.T) {
	t.Parallel()

	if got := Lookup(""); got != Lookup("full") {
		t.Fatalf("Lookup(empty) and Lookup(full) should match")
	}
	if got := Lookup("quickstart"); !strings.Contains(got, "Run a node:") {
		t.Fatalf("Lookup(quickstart) missing quickstart content")
	}
	if got := Lookup("rules"); !strings.HasPrefix(got, "# Embedded Rules") {
		t.Fatalf("Lookup(rules) = %q, want embedded rules", got)
	}
	if got := Lookup("protocol"); !strings.HasPrefix(got, "# KS Envelope") {
		t.Fatalf("Lookup(protocol) = %q, want KS Envelope topic", got)
	}
	if got := Protocol(); !strings.HasPrefix(got, "# Lightweight Protocol") {
		t.Fatalf("Protocol() = %q, want lightweight protocol", got)
	}
	if Lookup("protocol") == Protocol() {
		t.Fatal("Lookup(protocol) should return the topic-map protocol text, not the standalone protocol text")
	}
	if got := Lookup("phase=2"); got != Phases() {
		t.Fatal("Lookup(phase=*) should return the phase map")
	}
	if got := Lookup("phases"); got != Phases() {
		t.Fatal("Lookup(phases) should return the phase map")
	}
	if got := Lookup("unknown topic"); !strings.Contains(got, "Topics:") {
		t.Fatalf("Lookup(fallback) = %q, want generic help text", got)
	}
}
