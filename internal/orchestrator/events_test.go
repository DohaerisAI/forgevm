package orchestrator

import (
	"testing"
	"time"
)

func TestEventBus_SubscribePublish(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("test-sub")

	bus.Publish(Event{
		Type:      EventSandboxCreated,
		SandboxID: "sb-001",
	})

	select {
	case evt := <-ch:
		if evt.Type != EventSandboxCreated {
			t.Fatalf("expected sandbox.created, got %s", evt.Type)
		}
		if evt.SandboxID != "sb-001" {
			t.Fatalf("expected sb-001, got %s", evt.SandboxID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}

	bus.Unsubscribe("test-sub")
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe("test-sub")
	bus.Unsubscribe("test-sub")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Fatal("expected closed channel")
	}
}

func TestEventBus_NonBlockingPublish(t *testing.T) {
	bus := NewEventBus()
	// Subscribe but never read — channel buffer will fill
	bus.Subscribe("slow")

	// Publish more than buffer size — should not block
	done := make(chan struct{})
	go func() {
		for i := 0; i < defaultSubscriberBuf+10; i++ {
			bus.Publish(Event{Type: EventExecStarted})
		}
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked")
	}

	bus.Unsubscribe("slow")
}

func TestEventBus_History(t *testing.T) {
	bus := NewEventBus()

	for i := 0; i < 5; i++ {
		bus.Publish(Event{Type: EventSandboxCreated})
	}

	h := bus.History(3)
	if len(h) != 3 {
		t.Fatalf("expected 3 events, got %d", len(h))
	}

	all := bus.History(0)
	if len(all) != 5 {
		t.Fatalf("expected 5 events, got %d", len(all))
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	ch1 := bus.Subscribe("sub1")
	ch2 := bus.Subscribe("sub2")

	bus.Publish(Event{Type: EventSandboxDestroyed, SandboxID: "sb-002"})

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case evt := <-ch:
			if evt.SandboxID != "sb-002" {
				t.Fatalf("expected sb-002, got %s", evt.SandboxID)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}

	bus.Unsubscribe("sub1")
	bus.Unsubscribe("sub2")
}
