package clients

import (
	"sync"
	"testing"
	"time"
)

// Regression for review finding GB-4: AddMetadata wrote to a nil map.
func TestMetadata(t *testing.T) {
	c := New()
	if c.GetMetadata("missing") != nil {
		t.Fatal("missing key should be nil")
	}
	c.AddMetadata("map", 3)
	if c.GetMetadata("map") != 3 {
		t.Fatal("metadata not stored")
	}
}

func TestUUID(t *testing.T) {
	first, second := New(), New()
	if first.GetUUID() == second.GetUUID() {
		t.Fatal("default UUIDs should differ")
	}
	if New(WithUUID("abc")).GetUUID() != "abc" {
		t.Fatal("WithUUID ignored")
	}
}

func TestLastMessage(t *testing.T) {
	c := New()
	if !c.GetLastMessage().IsZero() {
		t.Fatal("new client should have no last message")
	}
	before := time.Now()
	c.SetLastMessageNow()
	if c.GetLastMessage().Before(before) {
		t.Fatal("SetLastMessageNow not recorded")
	}
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	c.SetLastMessage(at)
	if !c.GetLastMessage().Equal(at) {
		t.Fatal("SetLastMessage not recorded")
	}
	c.SetLastMessage(time.Time{})
	if !c.GetLastMessage().IsZero() {
		t.Fatal("zero time should reset")
	}
}

// Regression for GB-3: lastMessage and metadata were unsynchronised.
// Run with -race.
func TestConcurrentUse(t *testing.T) {
	c := New()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for range 100 {
				c.SetLastMessageNow()
				_ = c.GetLastMessage()
				c.AddMetadata("k", i)
				_ = c.GetMetadata("k")
			}
		})
	}
	wg.Wait()
}
