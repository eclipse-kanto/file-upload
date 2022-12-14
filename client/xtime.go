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
	"log"
	"time"
)

// Xtime is custom stuct, containing a time.Time in order to add json unmarshal support
type Xtime struct {
	Time *time.Time
}

// UnmarshalJSON unmarshal Xtime type
func (t *Xtime) UnmarshalJSON(b []byte) error {
	// Ignore null, like in the main JSON package.
	if string(b) == "null" {
		return nil
	}
	result, err := time.Parse(`"`+time.RFC3339+`"`, string(b))
	if err != nil {
		log.Fatalf("Failed to parse time flag: %v\n", err)
		return err
	}
	*t = Xtime{&result}
	return nil
}

// Set Xtime from string, used for flag set
func (t *Xtime) Set(s string) error {
	if s == "" {
		return nil
	}
	result, err := time.Parse(time.RFC3339, s)
	if err != nil {
		log.Fatalf("Failed to parse time flag: %v\n", err)
		return err
	}
	*t = Xtime{&result}
	return nil
}

func (t Xtime) String() string {
	if t.Time != nil {
		return t.Time.Format(time.RFC3339)
	}
	return ""
}
