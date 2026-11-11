package sse

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestReaderHandlesFragmentedMultilineEventsAndEOF(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		input := "event: first" + ending + ": keepalive" + ending + "data:{" + ending + "data: \"value\": 1}" + ending + ending + "data:tail"
		reader := NewReader(iotest.OneByteReader(strings.NewReader(input)))
		first, err := reader.Next()
		if err != nil || first.Name != "first" || first.Data != "{\n\"value\": 1}" {
			t.Fatalf("ending=%q event=%#v err=%v", ending, first, err)
		}
		last, err := reader.Next()
		if err != nil || last.Name != "" || last.Data != "tail" {
			t.Fatalf("ending=%q event=%#v err=%v", ending, last, err)
		}
		if _, err := reader.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("ending=%q err=%v, want EOF", ending, err)
		}
	}
}

func TestReaderKeepsReadErrorAfterCompleteEvents(t *testing.T) {
	source := iotest.ErrReader(io.ErrUnexpectedEOF)
	reader := NewReader(io.MultiReader(strings.NewReader("data: ok\n\n"), source))
	if event, err := reader.Next(); err != nil || event.Data != "ok" {
		t.Fatalf("event=%#v err=%v", event, err)
	}
	if _, err := reader.Next(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err=%v, want original read error", err)
	}
}

func TestReaderFlushesFinalEventBeforeUnexpectedEOF(t *testing.T) {
	for _, ending := range []string{"", "\n", "\r\n", "\r"} {
		for _, withData := range []bool{false, true} {
			t.Run(fmt.Sprintf("ending=%q/error_with_data=%t", ending, withData), func(t *testing.T) {
				readErr := fmt.Errorf("truncated HTTP body: %w", io.ErrUnexpectedEOF)
				var source io.Reader = io.MultiReader(
					strings.NewReader("data: first\n\nevent: last\ndata: {\ndata: \"value\": 1}"+ending),
					iotest.ErrReader(readErr),
				)
				if withData {
					source = iotest.DataErrReader(source)
				}
				reader := NewReader(source)
				if event, err := reader.Next(); err != nil || event.Data != "first" {
					t.Fatalf("first event=%#v err=%v", event, err)
				}
				if event, err := reader.Next(); err != nil || event.Name != "last" || event.Data != "{\n\"value\": 1}" {
					t.Fatalf("last event=%#v err=%v", event, err)
				}
				if _, err := reader.Next(); !errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
					t.Fatalf("err=%v, want original unexpected EOF", err)
				}
			})
		}
	}
}

func TestReaderAcceptsLargeToolEventAndRejectsOverflow(t *testing.T) {
	large := strings.Repeat("x", DefaultMaxEventBytes+1)
	reader := NewReader(strings.NewReader("data: " + large + "\n\n"))
	if event, err := reader.Next(); err != nil || event.Data != large {
		t.Fatalf("large conversion event was lost: length=%d err=%v", len(event.Data), err)
	}
	reader = NewReader(strings.NewReader("data: " + strings.Repeat("x", 100)))
	reader.parser = NewParser(64)
	if _, err := reader.Next(); !errors.Is(err, ErrEventTooLarge) {
		t.Fatalf("err=%v, want explicit event size error", err)
	}
}
