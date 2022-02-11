// Copyright (c) 2021 Contributors to the Eclipse Foundation
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
	"sync/atomic"
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	tickTime := int64(-1)
	end := sync.WaitGroup{}
	end.Add(1)

	start := time.Now().Add(time.Second)
	e := NewPeriodicExecutor(&start, nil, time.Millisecond*50, func() {
		swapped := atomic.CompareAndSwapInt64(&tickTime, -1, time.Now().UnixNano())
		if swapped {
			end.Done()
		}
	})
	defer e.Stop()

	end.Wait()

	if tickTime < start.Unix() {
		t.Fatalf("first tick time - %v - is before the start time - %v", time.Unix(0, tickTime), start)
	}
}

func TestEnd(t *testing.T) {
	var tickTime atomic.Value

	const period = time.Millisecond * 200
	end := time.Now().Add(time.Second)
	e := NewPeriodicExecutor(nil, &end, period, func() {
		tickTime.Store(time.Now())
	})
	defer e.Stop()

	time.Sleep(2 * time.Second)

	threshold := end.Add(period)
	if tickTime.Load().(time.Time).After(threshold) {
		t.Fatalf("last tick time - %v - is after the end time - %v", tickTime, end)
	}
}

func TestStop(t *testing.T) {
	var tickTime atomic.Value
	e := NewPeriodicExecutor(nil, nil, 200*time.Millisecond, func() {
		t := time.Now()
		tickTime.Store(&t)
	})

	time.Sleep(time.Second)
	e.Stop()
	end := time.Now().Add(50 * time.Millisecond)

	time.Sleep(time.Second)

	tt := tickTime.Load().(*time.Time)
	if tt == nil {
		t.Fatal("at least one tick expected, but there were none")
	}

	if tt.After(end) {
		t.Fatalf("tick received at %v, after end time %v", tickTime, end)
	}
}

func TestTicks(t *testing.T) {
	start := time.Now().Add(time.Second)
	end := start.Add(time.Second)
	const period = 200 * time.Millisecond

	c := int32(0)
	e := NewPeriodicExecutor(&start, &end, period, func() {
		atomic.AddInt32(&c, 1)
	})
	defer e.Stop()

	time.Sleep(time.Until(end) + time.Millisecond*500)

	expected := int(time.Second / period)

	c = atomic.LoadInt32(&c)
	if c < int32(expected) {
		t.Fatalf("too few ticks received - expected %d, but were %d", c, expected)
	}

	if c > int32(float32(expected)*1.5) {
		t.Fatalf("too many ticks received - expected %d, but were %d", c, expected)

	}
}
