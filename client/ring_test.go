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

	r := NewRingBuffer(size)

	exp := make([]interface{}, size)
	for i := 0; i < size; i++ {
		exp[i] = i
	}

	r.Put(exp...)

	act := make([]interface{}, size)
	for i := 0; i < size; i++ {
		act[i] = r.Get()
	}

	assertEquals(t, exp, act)
	if !r.Empty() {
		t.Fatalf("expected buffer to be empty")
	}

	r.Put(exp...)
	act = getElements(r)

	assertEquals(t, exp, act)
}

func TestRingBufferOverwrite(t *testing.T) {
	size := rand.Intn(100) + 1
	overwrite := rand.Intn(100) + 1

	t.Logf("TestRingBufferOverwrite with size %d and overwrite %d", size, overwrite)

	r := NewRingBuffer(size)

	src := make([]interface{}, size+overwrite)
	for i := range src {
		src[i] = i
	}

	r.Put(src...)

	exp := src[overwrite:]
	act := getElements(r)

	assertEquals(t, exp, act)
}

func TestRingBufferPanic(t *testing.T) {
	size := rand.Intn(100) + 1

	t.Logf("TestRingBufferPanic with size %d", size)

	r := NewRingBuffer(size)

	getFunc := func() {
		r.Get()
	}

	assertPanic(t, getFunc)

	r.Put(1)
	r.Get()

	assertPanic(t, getFunc)

	for i := 0; i < size; i++ {
		r.Put(i)
	}

	for i := 0; i < size; i++ {
		r.Get()
	}

	assertPanic(t, getFunc)
}

func getElements(r *RingBuffer) []interface{} {
	e := make([]interface{}, 0)

	for !r.Empty() {
		e = append(e, r.Get())
	}

	return e
}

func assertEquals(t *testing.T, exp interface{}, act interface{}) {
	t.Helper()

	if !reflect.DeepEqual(exp, act) {
		t.Fatalf("expected %v, but was %v", exp, act)
	}
}

func assertPanic(t *testing.T, f func()) {
	t.Helper()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("panic expected, but there was none")
		}
	}()

	f()
}
