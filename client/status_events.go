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

// statusEventsConsumer uses a bounded cyclic events queue, safe for concurrent use.
// When capacity is reached, oldest events in the queue are replaced by newer ones.
type statusEventsConsumer struct {
	buf *ringBuffer

	closed bool
	cond   *sync.Cond
}

// newStatusEventsConsumer constructs a new statusEventsConsumer with the given size.
func newStatusEventsConsumer(size int) *statusEventsConsumer {
	consumer := &statusEventsConsumer{}

	consumer.buf = newRingBuffer(size)
	consumer.cond = sync.NewCond(&sync.Mutex{})

	return consumer
}

// start event delivery loop.
func (q *statusEventsConsumer) start(consume func(e interface{})) {
	go q.eventLoop(consume)
}

// stop event delivery loop.
func (q *statusEventsConsumer) stop() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.closed = true

	q.cond.Broadcast()
}

// add new event to the queue.
func (q *statusEventsConsumer) add(e interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.buf.put(e)

	q.cond.Broadcast()
}

func (q *statusEventsConsumer) eventLoop(consume func(e interface{})) {
	for {
		if e, ok := q.get(); ok {
			consume(e)
		} else {
			break
		}
	}
}

func (q *statusEventsConsumer) get() (interface{}, bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	for q.buf.empty() && !q.closed {
		q.cond.Wait()
	}

	if q.closed {
		return nil, false
	}

	return q.buf.get(), true
}
