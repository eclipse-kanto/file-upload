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
	"math/rand"
	"reflect"
	"testing"
)

func TestRingBufferBasic(t *testing.T) {
	size := rand.Intn(100) + 1

	t.Logf("TestRingBufferBasic with size %d", size)

	r := newRingBuffer(size)

	exp := make([]interface{}, size)
	for i := 0; i < size; i++ {
		exp[i] = i
	}

	r.put(exp...)

	act := make([]interface{}, size)
	for i := 0; i < size; i++ {
		act[i], _ = r.get()
	}

	assertEquals(t, exp, act)
	if !r.empty() {
		t.Fatalf("expected buffer to be empty")
	}

	r.put(exp...)
	act = getElements(r)

	assertEquals(t, exp, act)
}

func TestRingBufferOverwrite(t *testing.T) {
	size := rand.Intn(100) + 1
	overwrite := rand.Intn(100) + 1

	t.Logf("TestRingBufferOverwrite with size %d and overwrite %d", size, overwrite)

	r := newRingBuffer(size)

	src := make([]interface{}, size+overwrite)
	for i := range src {
		src[i] = i
	}

	r.put(src...)

	exp := src[overwrite:]
	act := getElements(r)

	assertEquals(t, exp, act)
}

func TestRingBufferError(t *testing.T) {
	size := rand.Intn(100) + 1

	t.Logf("TestRingBufferPanic with size %d", size)

	r := newRingBuffer(size)

	el, exists := r.get()
	assertEquals(t, nil, el)
	assertEquals(t, false, exists)

	r.put(1)
	r.get()

	el, exists = r.get()
	assertEquals(t, nil, el)
	assertEquals(t, false, exists)

	for i := 0; i < size; i++ {
		r.put(i)
	}

	for i := 0; i < size; i++ {
		_, exists := r.get()
		assertEquals(t, true, exists)
	}

	el, exists = r.get()
	assertEquals(t, nil, el)
	assertEquals(t, false, exists)

}

func getElements(r *ringBuffer) []interface{} {
	e := make([]interface{}, 0)

	for !r.empty() {
		el, _ := r.get()
		e = append(e, el)
	}

	return e
}

func assertEquals(t *testing.T, exp interface{}, act interface{}) {
	t.Helper()

	if !reflect.DeepEqual(exp, act) {
		t.Fatalf("expected %v, but was %v", exp, act)
	}
}
