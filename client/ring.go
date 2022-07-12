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

// RingBuffer is a circular buffer of interface{} elements with fixed capacity.
// Oldest elements are overwritten, when there is no more space in the buffer.
type RingBuffer struct {
	elements []interface{}
	start    int
	end      int
}

// NewRingBuffer creates a new RingBuffer with the given capacity
func NewRingBuffer(capacity int) *RingBuffer {
	buf := &RingBuffer{}

	buf.elements = make([]interface{}, capacity+1) //+1 for sentinel

	return buf
}

// Empty returns true, if the buffer is empty
func (buf *RingBuffer) Empty() bool {
	return buf.start == buf.end
}

// Get the element at the head of the buffer, panic if empty
func (buf *RingBuffer) Get() interface{} {
	if buf.Empty() {
		panic("getting from empty buffer")
	}

	e := buf.elements[buf.start]
	buf.start = (buf.start + 1) % len(buf.elements)

	return e
}

// Put elements at the tail of the buffer,
// potentially overwriting oldest elements and moving the head of the buffer
func (buf *RingBuffer) Put(elements ...interface{}) {
	for _, e := range elements {
		buf.elements[buf.end] = e

		buf.end = (buf.end + 1) % len(buf.elements)

		if buf.end == buf.start {
			buf.start = (buf.start + 1) % len(buf.elements)
		}
	}
}
