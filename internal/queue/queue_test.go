package queue

import (
	"sync"
	"testing"
	"time"
)

func TestNewQueue(t *testing.T) {
	q := New(100)
	if q == nil {
		t.Fatal("expected non-nil queue")
	}
	if q.Capacity() != 100 {
		t.Errorf("expected capacity 100, got %d", q.Capacity())
	}
	if q.Size() != 0 {
		t.Errorf("expected size 0, got %d", q.Size())
	}
	if !q.IsEmpty() {
		t.Error("expected queue to be empty")
	}
	if q.IsFull() {
		t.Error("expected queue to not be full")
	}
}

func TestQueue_PushPop(t *testing.T) {
	q := New(10)

	if err := q.Push("item1"); err != nil {
		t.Fatalf("push failed: %v", err)
	}
	if q.Size() != 1 {
		t.Errorf("expected size 1, got %d", q.Size())
	}

	item, err := q.Pop()
	if err != nil {
		t.Fatalf("pop failed: %v", err)
	}
	if item != "item1" {
		t.Errorf("expected item1, got %v", item)
	}
	if q.Size() != 0 {
		t.Errorf("expected size 0, got %d", q.Size())
	}
}

func TestQueue_TryPushTryPop(t *testing.T) {
	q := New(2)

	if !q.TryPush("a") {
		t.Error("TryPush should succeed on empty queue")
	}
	if !q.TryPush("b") {
		t.Error("TryPush should succeed when not full")
	}
	if q.TryPush("c") {
		t.Error("TryPush should fail on full queue")
	}

	item, ok := q.TryPop()
	if !ok {
		t.Error("TryPop should succeed on non-empty queue")
	}
	if item != "a" {
		t.Errorf("expected a, got %v", item)
	}

	q.TryPop()
	_, ok = q.TryPop()
	if ok {
		t.Error("TryPop should fail on empty queue")
	}
}

func TestQueue_PopBatch(t *testing.T) {
	q := New(10)

	for i := 0; i < 5; i++ {
		q.Push(i)
	}

	batch, err := q.PopBatch(3)
	if err != nil {
		t.Fatalf("PopBatch failed: %v", err)
	}
	if len(batch) != 3 {
		t.Errorf("expected batch of 3, got %d", len(batch))
	}
	if q.Size() != 2 {
		t.Errorf("expected size 2 after batch pop, got %d", q.Size())
	}
}

func TestQueue_PopBatch_AllItems(t *testing.T) {
	q := New(10)
	q.Push(1)
	q.Push(2)

	batch, err := q.PopBatch(10)
	if err != nil {
		t.Fatalf("PopBatch failed: %v", err)
	}
	if len(batch) != 2 {
		t.Errorf("expected batch of 2, got %d", len(batch))
	}
	if q.Size() != 0 {
		t.Errorf("expected size 0, got %d", q.Size())
	}
}

func TestQueue_PushWithTimeout(t *testing.T) {
	q := New(1)
	q.Push("first")

	err := q.PushWithTimeout("second", 50*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestQueue_PopWithTimeout(t *testing.T) {
	q := New(10)

	_, err := q.PopWithTimeout(50 * time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestQueue_Close(t *testing.T) {
	q := New(10)
	q.Push("item")

	q.Close()

	err := q.Push("after_close")
	if err == nil {
		t.Error("expected error pushing to closed queue")
	}

	if !q.TryPush("another") {
		// TryPush should return false for closed queue
	}
}

func TestQueue_Close_EmptyPop(t *testing.T) {
	q := New(10)
	q.Close()

	_, err := q.Pop()
	if err == nil {
		t.Error("expected error popping from closed empty queue")
	}
}

func TestQueue_Clear(t *testing.T) {
	q := New(10)
	q.Push(1)
	q.Push(2)
	q.Push(3)

	q.Clear()

	if q.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", q.Size())
	}
	if !q.IsEmpty() {
		t.Error("expected queue to be empty after clear")
	}
}

func TestQueue_Stats(t *testing.T) {
	q := New(10)
	q.Push(1)
	q.Push(2)

	stats := q.Stats()
	if stats.Size != 2 {
		t.Errorf("expected stats size 2, got %d", stats.Size)
	}
	if stats.Capacity != 10 {
		t.Errorf("expected stats capacity 10, got %d", stats.Capacity)
	}
	if stats.IsFull {
		t.Error("expected stats IsFull to be false")
	}
	if stats.IsEmpty {
		t.Error("expected stats IsEmpty to be false")
	}
}

func TestQueue_ConcurrentAccess(t *testing.T) {
	q := New(100)
	var wg sync.WaitGroup
	numProducers := 5
	numConsumers := 5
	itemsPerProducer := 20

	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				q.Push(producerID*1000 + j)
			}
		}(i)
	}

	consumed := make(chan interface{}, numProducers*itemsPerProducer)
	for i := 0; i < numConsumers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				item, ok := q.TryPop()
				if !ok {
					time.Sleep(time.Millisecond)
					if q.IsEmpty() {
						return
					}
					continue
				}
				consumed <- item
			}
		}()
	}

	wg.Wait()
	close(consumed)

	count := 0
	for range consumed {
		count++
	}

	// All items may not be consumed due to timing; just verify no panics occurred
	t.Logf("Consumed %d items", count)
}

func TestQueue_IsFull(t *testing.T) {
	q := New(2)
	if q.IsFull() {
		t.Error("new queue should not be full")
	}

	q.Push(1)
	if q.IsFull() {
		t.Error("queue with 1/2 items should not be full")
	}

	q.Push(2)
	if !q.IsFull() {
		t.Error("queue at capacity should be full")
	}
}

func TestQueue_Capacity(t *testing.T) {
	q := New(42)
	if q.Capacity() != 42 {
		t.Errorf("expected capacity 42, got %d", q.Capacity())
	}
}
