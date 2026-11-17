package logs

import (
	"log"
	"strings"
	"testing"
)

func TestInstallCapturesAndPreservesConsoleOutput(t *testing.T) {
	var console strings.Builder
	Install(&console)
	t.Cleanup(func() { Install(nil) })
	log.Printf("[测试] 上游拉取失败")

	if !strings.Contains(console.String(), "上游拉取失败") {
		t.Fatalf("console output lost: %q", console.String())
	}
	entries := Recent(10, 0)
	if len(entries) == 0 {
		t.Fatal("expected captured entry")
	}
	last := entries[len(entries)-1]
	if !strings.Contains(last.Message, "上游拉取失败") {
		t.Fatalf("message = %q", last.Message)
	}
	if last.Level != "error" {
		t.Fatalf("level = %q, want error", last.Level)
	}
	if strings.Contains(last.Message, "2026/") {
		t.Fatalf("message still carries the log timestamp prefix: %q", last.Message)
	}
}

func TestRecentFiltersBySequence(t *testing.T) {
	Install(nil)
	t.Cleanup(func() { Install(nil) })
	log.SetFlags(0)
	log.Printf("first")
	entries := Recent(1, 0)
	if len(entries) != 1 {
		t.Fatalf("buffer = %#v", entries)
	}
	seq := entries[0].Seq
	log.Printf("second")
	after := Recent(10, seq)
	if len(after) != 1 || after[0].Message != "second" {
		t.Fatalf("after = %#v, want only the newer line", after)
	}
	if after[0].Level != "info" {
		t.Fatalf("level = %q, want info", after[0].Level)
	}
}

func TestSubscribeReceivesPublishedEntries(t *testing.T) {
	Install(nil)
	t.Cleanup(func() { Install(nil) })
	log.SetFlags(0)
	entries, cancel := Subscribe()
	defer cancel()

	log.Printf("[警告] 出口切换")

	select {
	case entry := <-entries:
		if entry.Level != "warn" {
			t.Fatalf("level = %q, want warn", entry.Level)
		}
		if entry.Message != "[警告] 出口切换" {
			t.Fatalf("message = %q", entry.Message)
		}
	default:
		t.Fatal("subscriber did not receive the published line")
	}
}
