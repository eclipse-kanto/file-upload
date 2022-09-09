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

// ringBuffer is a circular buffer of interface{} elements with fixed capacity.
// Oldest elements are overwritten, when there is no more space in the buffer.
type ringBuffer struct {
	elements []interface{}
	start    int
	end      int
}

// newRingBuffer creates a new ringBuffer with the given size.
func newRingBuffer(size int) *ringBuffer {
	buf := &ringBuffer{}

	buf.elements = make([]interface{}, size+1)

	return buf
}

// empty returns true, if the buffer is empty.
func (buf *ringBuffer) empty() bool {
	return buf.start == buf.end
}

// get the element at the beginning of the buffer, if such exists.
func (buf *ringBuffer) get() (interface{}, bool) {
	if buf.empty() {
		return nil, false
	}

	e := buf.elements[buf.start]
	buf.start = (buf.start + 1) % len(buf.elements)

	return e, true
}

// put elements at the end of the buffer,
// potentially overwriting oldest elements and moving the beginning of the buffer.
func (buf *ringBuffer) put(elements ...interface{}) {
	for _, e := range elements {
		buf.elements[buf.end] = e

		buf.end = (buf.end + 1) % len(buf.elements)

		if buf.end == buf.start {
			buf.start = (buf.start + 1) % len(buf.elements)
		}
	}
}
