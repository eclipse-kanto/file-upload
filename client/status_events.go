// Copyright (c) 2022 Contributors to the Eclipse Foundation
//
// See the NOTICE file(s) distributed with this work for additional
// information regarding copyright ownership.
//
// This program and the accompanying materials are made available under the
// terms of the Eclipse Public License 2.0 which is available at
// https://www.eclipse.org/legal/epl-2.0, or the Apache License, Version 2.0
// which is available at https://www.apache.org/licenses/LICENSE-2.0.
//
// SPDX-License-Identifier: EPL-2.0 OR Apache-2.0

package client

import (
	"sync"
)

// StatusEventsConsumer uses a bounded cyclic events queue, safe for concurrent use.
// When capacity is reached, oldest events in the queue are replaced by newer ones.
type StatusEventsConsumer struct {
	buf *ringBuffer

	closed bool
	cond   *sync.Cond
}

// NewStatusEventsConsumer constructs a new StatusEventsConsumer with the given size.
func NewStatusEventsConsumer(size int) *StatusEventsConsumer {
	consumer := &StatusEventsConsumer{}

	consumer.buf = newRingBuffer(size)
	consumer.cond = sync.NewCond(&sync.Mutex{})

	return consumer
}

// Start event delivery loop.
func (q *StatusEventsConsumer) Start(consume func(e interface{})) {
	q.closed = false
	go q.eventLoop(consume)
}

// Stop event delivery loop.
func (q *StatusEventsConsumer) Stop() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.closed = true

	q.cond.Broadcast()
}

// Add new event to the queue.
func (q *StatusEventsConsumer) Add(e interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.buf.put(e)

	q.cond.Broadcast()
}

func (q *StatusEventsConsumer) eventLoop(consume func(e interface{})) {
	for {
		if e, ok := q.get(); ok {
			consume(e)
		} else {
			break
		}
	}
}

func (q *StatusEventsConsumer) get() (interface{}, bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	for q.buf.empty() && !q.closed {
		q.cond.Wait()
	}

	if q.closed {
		return nil, false
	}

	return q.buf.get()
}
