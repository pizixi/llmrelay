package sse

import "testing"

func TestParserSmoke(t *testing.T) {
	p := NewParser(DefaultMaxEventBytes)
	input := []byte("data: {\"usage\":{\"prompt_tokens\":7}}\n\ndata: [DONE]\n")
	got := p.Feed(input)
	got = append(got, p.Flush()...)
	if len(got) != 2 {
		t.Fatalf("events=%#v", got)
	}
	if got[0].Data != "{\"usage\":{\"prompt_tokens\":7}}" || got[1].Data != "[DONE]" {
		t.Fatalf("events=%#v", got)
	}
}

func TestParserHandlesChunkBoundariesAndSSEFields(t *testing.T) {
	p := NewParser(DefaultMaxEventBytes)
	var events []Event
	for _, chunk := range [][]byte{
		[]byte("event: response.completed\r\ndata: {\"a\":"),
		[]byte("1}\r\ndata: {\"b\":2}\r\n\r\n: observer\r\n"),
		[]byte("data: tail\r\r"),
	} {
		events = append(events, p.Feed(chunk)...)
	}
	events = append(events, p.Flush()...)
	if len(events) != 2 {
		t.Fatalf("events=%#v", events)
	}
	if events[0].Name != "response.completed" || events[0].Data != "{\"a\":1}\n{\"b\":2}" {
		t.Fatalf("first event=%#v", events[0])
	}
	if events[1].Data != "tail" {
		t.Fatalf("tail event=%#v", events[1])
	}
}

func TestParserStopsObservingOversizedEvent(t *testing.T) {
	p := NewParser(8)
	if got := p.Feed([]byte("data: this is too large\n\n")); len(got) != 0 || !p.Dropped() {
		t.Fatalf("events=%#v dropped=%v", got, p.Dropped())
	}
	if got := p.Feed([]byte("data: {\"ok\":true}\n\n")); len(got) != 0 {
		t.Fatalf("dropped parser emitted events=%#v", got)
	}
}

func TestParserDropsUnterminatedOversizedLine(t *testing.T) {
	p := NewParser(8)
	if got := p.Feed([]byte("data: ")); len(got) != 0 || p.Dropped() {
		t.Fatalf("initial events=%#v dropped=%v", got, p.Dropped())
	}
	if got := p.Feed([]byte("this line never ends")); len(got) != 0 || !p.Dropped() {
		t.Fatalf("events=%#v dropped=%v", got, p.Dropped())
	}
}
