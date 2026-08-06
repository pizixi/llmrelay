package state

import (
	"testing"
	"time"
)

func TestStoreRoundTripAndOutputIsolation(t *testing.T) {
	store := New(2, time.Hour)
	response := map[string]any{
		"id":     "resp_1",
		"output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "hello"}}}},
	}
	if id, ok := store.PutResponse(response); !ok || id != "resp_1" {
		t.Fatalf("put id=%q ok=%v", id, ok)
	}
	items, ok := store.ResolveOutputItems("resp_1")
	if !ok || len(items) != 1 {
		t.Fatalf("items=%#v ok=%v", items, ok)
	}
	item := items[0].(map[string]any)
	item["type"] = "mutated"
	stored, ok := store.Get("resp_1")
	if !ok || stored["output"].([]any)[0].(map[string]any)["type"] != "message" {
		t.Fatalf("store leaked mutable output: %#v", stored)
	}
	if _, ok := store.PutResponse(map[string]any{"output": []any{}}); ok {
		t.Fatal("response without id was stored")
	}
}

func TestStoreEvictsAndExpires(t *testing.T) {
	store := New(1, 10*time.Millisecond)
	store.PutResponse(map[string]any{"id": "first", "output": []any{map[string]any{"type": "message"}}})
	store.PutResponse(map[string]any{"id": "second", "output": []any{map[string]any{"type": "message"}}})
	if _, ok := store.Get("first"); ok {
		t.Fatal("oldest entry was not evicted")
	}
	time.Sleep(15 * time.Millisecond)
	if _, ok := store.Get("second"); ok {
		t.Fatal("expired entry was returned")
	}
}
