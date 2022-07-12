// Copyright (c) 2022 Contributors to the Eclipse Foundation
//
// See the NOTICE file(s) distributed with this work for additional
// information regarding copyright ownership.
//
// This program and the accompanying materials are made available under the
// terms of the Eclipse Public License 2.0 which is available at
// http://www.eclipse.org/legal/epl-2.0
//
// SPDX-License-Identifier: EPL-2.0

package client

import (
	"sync"
)

// EventsQueue is a bounded cyclic events queue, safe for concurrent use.
// When capacity is reached, oldest events in the queue are replaced by newer ones.
type EventsQueue struct {
	buf *RingBuffer

	closed bool
	cond   *sync.Cond
}

// NewEventsQueue constructs a new EventsQueue with given capacity
func NewEventsQueue(size int) *EventsQueue {
	queue := &EventsQueue{}

	queue.buf = NewRingBuffer(size)
	queue.cond = sync.NewCond(&sync.Mutex{})

	return queue
}

// Start event delivery loop
func (q *EventsQueue) Start(consume func(e interface{})) {
	go q.eventLoop(consume)
}

// Stop event delivery loop
func (q *EventsQueue) Stop() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.closed = true

	q.cond.Broadcast()
}

// Add new event to the queue.
func (q *EventsQueue) Add(e interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.buf.Put(e)

	q.cond.Broadcast()
}

func (q *EventsQueue) eventLoop(consume func(e interface{})) {
	for {
		if e, ok := q.get(); ok {
			consume(e)
		} else {
			break
		}
	}
}

func (q *EventsQueue) get() (interface{}, bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	for q.buf.Empty() && !q.closed {
		q.cond.Wait()
	}

	if q.closed {
		return nil, false
	}

	return q.buf.Get(), true
}
