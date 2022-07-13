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
		act[i] = r.get()
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

	getFunc := func() {
		r.get()
	}

	assertNoPanic(t, getFunc)

	r.put(1)
	r.get()

	assertNoPanic(t, getFunc)

	for i := 0; i < size; i++ {
		r.put(i)
	}

	for i := 0; i < size; i++ {
		r.get()
	}

	assertNoPanic(t, getFunc)
}

func getElements(r *ringBuffer) []interface{} {
	e := make([]interface{}, 0)

	for !r.empty() {
		e = append(e, r.get())
	}

	return e
}

func assertEquals(t *testing.T, exp interface{}, act interface{}) {
	t.Helper()

	if !reflect.DeepEqual(exp, act) {
		t.Fatalf("expected %v, but was %v", exp, act)
	}
}

func assertNoPanic(t *testing.T, f func()) {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("no panic is expected here, but there was - %v", r)
		}
	}()

	f()
}
