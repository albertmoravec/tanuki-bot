package main

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

type Queue struct {
	sync.RWMutex
	queue []*QueueItem
}

type Playable interface {
	Play() io.Reader
	Stop()
	GetInfo() ItemInfo
}

type ItemInfo struct {
	Title    string
	Duration string
}

type QueueItem struct {
	Stream      Playable
	Info        ItemInfo
	RequestedBy string
}

type ErrItemNotFound struct {
	item int
}

func (err ErrItemNotFound) Error() string {
	return fmt.Sprintf("Item number %d not found", err.item+1)
}

func (q *Queue) Add(items ...*QueueItem) {
	q.Lock()
	defer q.Unlock()

	q.queue = append(q.queue, items...)
}

func (q *Queue) Remove(i int) error {
	q.Lock()
	defer q.Unlock()

	if len(q.queue) == 0 {
		return errors.New("Queue is empty")
	} else if len(q.queue) <= i {
		return ErrItemNotFound{i}
	} else if len(q.queue) > 1 {
		copy(q.queue[i:], q.queue[i+1:])
		q.queue[len(q.queue)-1] = nil
		q.queue = q.queue[:len(q.queue)-1]
	} else {
		q.queue = nil
	}

	return nil
}

func (q *Queue) Purge() {
	q.Lock()
	defer q.Unlock()

	q.queue = nil
}

func (q *Queue) GetFirst() (*QueueItem, error) {
	q.RLock()
	defer q.RUnlock()

	if len(q.queue) > 0 {
		return q.queue[0], nil
	} else {
		return nil, errors.New("Queue is empty")
	}
}

func (q *Queue) Get(i int) (*QueueItem, error) {
	q.RLock()
	defer q.RUnlock()

	if len(q.queue) <= i {
		return nil, errors.New("Item doesn't exist")
	}

	return q.queue[i], nil
}

func (q *Queue) GetFirstN(n int) ([]*QueueItem, int, error) {
	q.RLock()
	defer q.RUnlock()

	if len(q.queue) == 0 {
		return nil, 0, errors.New("Queue is empty")
	}

	var queueCopy []*QueueItem
	var remaining int

	if len(q.queue) > n {
		queueCopy = make([]*QueueItem, n)
		copy(queueCopy, q.queue[:n])
		remaining = len(q.queue) - n
	} else {
		queueCopy = make([]*QueueItem, len(q.queue))
		copy(queueCopy, q.queue)
		remaining = 0
	}

	return queueCopy, remaining, nil
}

func (q *Queue) GetAll() ([]*QueueItem, error) {
	q.RLock()
	defer q.RUnlock()

	if len(q.queue) == 0 {
		return nil, errors.New("Queue is empty")
	}

	var queueCopy []*QueueItem
	copy(queueCopy, q.queue)

	return queueCopy, nil
}

func (q *Queue) Move(from int, to int) error {
	q.Lock()
	defer q.Unlock()

	if len(q.queue) == 0 {
		return errors.New("Queue is empty")
	} else if len(q.queue) <= from {
		return ErrItemNotFound{from}
	} else if from == to {
		return nil
	}

	item := q.queue[from]
	q.queue = append(q.queue[:from], q.queue[from+1:]...)

	if to == 0 {
		q.queue = append([]*QueueItem{item}, q.queue...)
	} else {
		q.queue = append(q.queue[:to], append([]*QueueItem{item}, q.queue[to:]...)...)
	}

	return nil
}
