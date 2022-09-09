// Copyright (c) 2021 Contributors to the Eclipse Foundation
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
	"time"
)

// PeriodicExecutor can be used to periodically executed given task in specified time frame.
type PeriodicExecutor struct {
	period time.Duration
	task   func()

	fromTimer *time.Timer
	toTimer   *time.Timer

	ticker *time.Ticker
	mutex  sync.Mutex
	done   chan bool
}

// NewPeriodicExecutor constructs a PeriodicExecutor for given time frame (from, to). The task function will be
// invoked at the specified period.
//
// The executor starts invoking the task when from time is reached. If from is nil of in the past, the executor
// starts right away. The execution continues till the to time is reached, unless to is nil. In that case execution
// continues until the Stop is invoked
func NewPeriodicExecutor(from *time.Time, to *time.Time, period time.Duration, task func()) *PeriodicExecutor {
	e := &PeriodicExecutor{}
	e.period = period
	e.task = task

	if from != nil {
		e.fromTimer = time.AfterFunc(time.Until(*from), func() {
			e.startTicker()
		})
	} else {
		e.startTicker()
	}

	if to != nil {
		e.toTimer = time.AfterFunc(time.Until(*to), func() {
			e.stopTicker()
		})
	}

	return e
}

func (e *PeriodicExecutor) startTicker() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.done = make(chan bool)
	e.ticker = time.NewTicker(e.period)

	go func() {
		e.task() //invoke at the start of the period

		defer func() {
			e.mutex.Lock()
			defer e.mutex.Unlock()

			e.ticker = nil
		}()

		for {
			select {
			case <-e.done:
				return
			case <-e.ticker.C:
				e.task()
			}
		}
	}()
}

func (e *PeriodicExecutor) stopTicker() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.ticker != nil {
		e.done <- true
	}
}

// Stop stops periodic execution and cleans used resources.
func (e *PeriodicExecutor) Stop() {
	e.stopTicker()

	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.fromTimer != nil {
		e.fromTimer.Stop()
		e.fromTimer = nil
	}

	if e.toTimer != nil {
		e.toTimer.Stop()
		e.toTimer = nil
	}

}
