package main

import "testing"

func TestInterpolateDefault(t *testing.T) {
	ev := apiGatewayEvent{Body: `{"start":{"dateTime":"16:00"}}`}
	got := interpolate(`${body.start.dateTime:-09:00}|${body.summary:-Budget review}|${path.id:-none}`, map[string]string{}, ev, "/x")
	want := "16:00|Budget review|none"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
