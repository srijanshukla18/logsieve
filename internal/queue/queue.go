package queue

import (
	"fmt"
	"sync"
	"time"
)

type Queue struct {
	items    []interface{}
	capacity int
	mutex    sync.RWMutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	closed   bool
}

type Stats struct {
	Size        int       `json:"size"`
	Capacity    int       `json:"capacity"`
	IsFull      bool      `json:"is_full"`
	IsEmpty     bool      `json:"is_empty"`
	LastUpdated time.Time `json:"last_updated"`
}

func New(capacity int) *Queue {
	q := &Queue{
		items:    make([]interface{}, 0, capacity),
		capacity: capacity,
	}
	q.notEmpty = sync.NewCond(&q.mutex)
	q.notFull = sync.NewCond(&q.mutex)
	return q
}

func (q *Queue) Push(item interface{}) error {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.closed {
		return fmt.Errorf("queue is closed")
	}

	for len(q.items) >= q.capacity && !q.closed {
		q.notFull.Wait()
	}

	if q.closed {
		return fmt.Errorf("queue is closed")
	}

	q.items = append(q.items, item)
	q.notEmpty.Signal()
	return nil
}

func (q *Queue) PushWithTimeout(item interface{}, timeout time.Duration) error {
	done := make(chan error, 1)
	
	go func() {
		done <- q.Push(item)
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("push timeout after %v", timeout)
	}
}

func (q *Queue) Pop() (interface{}, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for len(q.items) == 0 && !q.closed {
		q.notEmpty.Wait()
	}

	if len(q.items) == 0 && q.closed {
		return nil, fmt.Errorf("queue is closed and empty")
	}

	item := q.items[0]
	q.items = q.items[1:]
	q.notFull.Signal()
	return item, nil
}

func (q *Queue) PopWithTimeout(timeout time.Duration) (interface{}, error) {
	done := make(chan interface{}, 1)
	errChan := make(chan error, 1)
	
	go func() {
		item, err := q.Pop()
		if err != nil {
			errChan <- err
			return
		}
		done <- item
	}()

	select {
	case item := <-done:
		return item, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("pop timeout after %v", timeout)
	}
}

func (q *Queue) PopBatch(maxSize int) ([]interface{}, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	for len(q.items) == 0 && !q.closed {
		q.notEmpty.Wait()
	}

	if len(q.items) == 0 && q.closed {
		return nil, fmt.Errorf("queue is closed and empty")
	}

	batchSize := len(q.items)
	if batchSize > maxSize {
		batchSize = maxSize
	}

	batch := make([]interface{}, batchSize)
	copy(batch, q.items[:batchSize])
	q.items = q.items[batchSize:]
	
	q.notFull.Broadcast()
	return batch, nil
}

func (q *Queue) TryPop() (interface{}, bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if len(q.items) == 0 {
		return nil, false
	}

	item := q.items[0]
	q.items = q.items[1:]
	q.notFull.Signal()
	return item, true
}

func (q *Queue) TryPush(item interface{}) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.closed || len(q.items) >= q.capacity {
		return false
	}

	q.items = append(q.items, item)
	q.notEmpty.Signal()
	return true
}

func (q *Queue) Size() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return len(q.items)
}

func (q *Queue) Capacity() int {
	return q.capacity
}

func (q *Queue) IsEmpty() bool {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return len(q.items) == 0
}

func (q *Queue) IsFull() bool {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return len(q.items) >= q.capacity
}

func (q *Queue) Stats() Stats {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	
	return Stats{
		Size:        len(q.items),
		Capacity:    q.capacity,
		IsFull:      len(q.items) >= q.capacity,
		IsEmpty:     len(q.items) == 0,
		LastUpdated: time.Now(),
	}
}

func (q *Queue) Close() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	
	if q.closed {
		return
	}
	
	q.closed = true
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
}

func (q *Queue) Clear() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	
	q.items = q.items[:0]
	q.notFull.Broadcast()
}