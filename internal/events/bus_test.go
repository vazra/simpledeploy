package events

import (
	"context"
	"testing"
	"time"
)

func drain(ch <-chan Event, n int, timeout time.Duration) []Event {
	out := make([]Event, 0, n)
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for len(out) < n {
		select {
		case e, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, e)
		case <-deadline.C:
			return out
		}
	}
	return out
}

func TestBusFanOut(t *testing.T) {
	b := New()
	ch1, cancel1, _ := b.Subscribe(nil)
	defer cancel1()
	ch2, cancel2, _ := b.Subscribe(nil)
	defer cancel2()

	b.Publish(context.Background(), Event{Type: "x", Topic: "global:apps"})

	if got := drain(ch1, 1, 200*time.Millisecond); len(got) != 1 {
		t.Fatalf("ch1: got %d events", len(got))
	}
	if got := drain(ch2, 1, 200*time.Millisecond); len(got) != 1 {
		t.Fatalf("ch2: got %d events", len(got))
	}
}

func TestBusFilter(t *testing.T) {
	b := New()
	ch, cancel, _ := b.Subscribe(func(e Event) bool { return e.Topic == "app:foo" })
	defer cancel()

	b.Publish(context.Background(), Event{Topic: "app:bar"})
	b.Publish(context.Background(), Event{Topic: "app:foo"})

	got := drain(ch, 2, 100*time.Millisecond)
	if len(got) != 1 || got[0].Topic != "app:foo" {
		t.Fatalf("filter: got %+v", got)
	}
}

func TestBusOverflowSetsStale(t *testing.T) {
	b := New()
	ch, cancel, sub := b.Subscribe(nil)
	defer cancel()

	// Fill buffer + overflow.
	for i := 0; i < subBuffer+5; i++ {
		b.Publish(context.Background(), Event{Topic: "x"})
	}
	if !sub.Stale() {
		t.Fatal("expected stale=true after overflow")
	}
	// Drain to make sure channel still works.
	got := drain(ch, subBuffer, 200*time.Millisecond)
	if len(got) == 0 {
		t.Fatal("expected events drained")
	}
	sub.Reset()
	if sub.Stale() {
		t.Fatal("expected stale cleared after Reset")
	}
}

func TestBusUnsubscribe(t *testing.T) {
	b := New()
	ch, cancel, _ := b.Subscribe(nil)
	cancel()
	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected channel close")
	}
	// Publishing after unsubscribe must not panic.
	b.Publish(context.Background(), Event{Topic: "x"})
}

func TestTopicsForAudit(t *testing.T) {
	cases := []struct {
		cat, slug string
		want      []string
	}{
		{"compose", "foo", []string{"global:audit", "app:foo"}},
		{"lifecycle", "foo", []string{"global:audit", "app:foo", "global:apps"}},
		{"backup", "foo", []string{"global:audit", "app:foo", "global:backups"}},
		{"webhook", "", []string{"global:audit", "global:alerts"}},
		{"settings", "", []string{"global:audit", "global:settings"}},
		{"docker", "", []string{"global:audit", "global:docker"}},
	}
	for _, c := range cases {
		got := TopicsForAudit(c.cat, c.slug)
		if len(got) != len(c.want) {
			t.Fatalf("%s/%s: got %v, want %v", c.cat, c.slug, got, c.want)
		}
		for i, g := range got {
			if g != c.want[i] {
				t.Fatalf("%s/%s: got %v, want %v", c.cat, c.slug, got, c.want)
			}
		}
	}
}
