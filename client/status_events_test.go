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
	"sync/atomic"
	"testing"
	"time"
)

func TestStatusEventsConsumerDelivery(t *testing.T) {
	count := 10

	act := make([]interface{}, 0)
	var wg sync.WaitGroup

	statusEvents := newStatusEventsConsumer(count)
	statusEvents.start(func(e interface{}) {
		act = append(act, e)
		wg.Done()
	})

	exp := make([]interface{}, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		statusEvents.add(i)
		exp[i] = i
	}

	wg.Wait()

	assertEquals(t, exp, act)
}

func TestStatusEventsConsumerAdd(t *testing.T) {
	statusEvents := newStatusEventsConsumer(3)
	statusEvents.start(func(e interface{}) {
		time.Sleep(2 * time.Second)
	})

	start := time.Now()
	for i := 0; i < 10; i++ {
		statusEvents.add(i)
	}

	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Fatalf("add took %v - consume should not block add", elapsed)
	}
}

func TestStatusEventsConsumerClose(t *testing.T) {
	var counter int32
	statusEvents := newStatusEventsConsumer(3)
	statusEvents.start(func(e interface{}) {
		atomic.AddInt32(&counter, 1)
	})

	count := 5
	for i := 0; i < count; i++ {
		statusEvents.add("a")
	}

	statusEvents.stop()

	for i := 0; i < 10; i++ {
		statusEvents.add("b")
	}

	time.Sleep(1 * time.Second)

	act := atomic.LoadInt32(&counter)
	if act > int32(count) {
		t.Fatalf("expected at most %d events, but received %d", count, act)
	}
}

func TestStatusEventsConsumerOrdering(t *testing.T) {
	count := 50
	ls := make([]int, 0, count)

	var wg sync.WaitGroup
	statusEvents := newStatusEventsConsumer(count)
	statusEvents.start(func(e interface{}) {
		ls = append(ls, e.(int))
		wg.Done()
	})

	var latch sync.WaitGroup
	var m sync.Mutex
	counter := 0
	for i := 0; i < count; i++ {
		latch.Add(1)
		wg.Add(1)

		go func() {
			latch.Wait()

			m.Lock()
			defer m.Unlock()

			counter++
			statusEvents.add(counter)
		}()
	}
	latch.Add(-count)

	wg.Wait()

	for i := 0; i < count-1; i++ {
		if ls[i] >= ls[i+1] {
			t.Fatalf("incorrect ordering between - [%d] = %d vs [%d] = %d", i, ls[i], i+1, ls[i+1])
		}
	}
}
