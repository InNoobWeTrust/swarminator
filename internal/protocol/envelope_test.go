package protocol

import "testing"

func TestEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()

	original := NewEnvelope("reviewer", "challenge", "skill-review", "\n  Hello\nworld  \n")
	got := Parse(original.String())
	want := Envelope{
		Role:   "reviewer",
		Intent: "challenge",
		Target: "skill-review",
		Body:   "Hello\nworld",
	}
	if got != want {
		t.Fatalf("Parse(String()) = %#v, want %#v", got, want)
	}
}

func TestParseBodyOnly(t *testing.T) {
	t.Parallel()

	got := Parse("plain body only")
	want := Envelope{Body: "plain body only"}
	if got != want {
		t.Fatalf("Parse(body) = %#v, want %#v", got, want)
	}
}

func TestParseStopsAtMalformedHeader(t *testing.T) {
	t.Parallel()

	got := Parse("ROLE: reviewer\nnot-a-header\nrest of body")
	want := Envelope{
		Role: "reviewer",
		Body: "ROLE: reviewer\nnot-a-header\nrest of body",
	}
	if got != want {
		t.Fatalf("Parse(malformed) = %#v, want %#v", got, want)
	}
}
