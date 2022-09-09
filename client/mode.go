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

import (
	"encoding/json"
	"fmt"
)

// AccessMode type, for restricting files allowed to be dynamically requested for upload
type AccessMode int

// Allowed values for AccessMode
const (
	ModeNA = iota
	ModeStrict
	ModeLax
	ModeScoped
)

// AccessMode names
const (
	ModeNameStrict = "strict"
	ModeNameLax    = "lax"
	ModeNameScoped = "scoped"
)

// String returns string representation of AccessMode
func (m AccessMode) String() string {
	switch m {
	case ModeStrict:
		return ModeNameStrict
	case ModeLax:
		return ModeNameLax
	case ModeScoped:
		return ModeNameScoped
	default:
		return ""
	}
}

// Set implements flag.Value Set method
func (m *AccessMode) Set(v string) error {
	switch v {
	case ModeNameStrict:
		*m = ModeStrict
	case ModeNameLax:
		*m = ModeLax
	case ModeNameScoped:
		*m = ModeScoped
	case "":
		*m = ModeNA
	default:
		return fmt.Errorf("accepted values are '%s', '%s' and '%s'", ModeNameStrict, ModeNameLax, ModeNameScoped)
	}

	return nil
}

// MarshalJSON marshals AccessMode as JSON
func (m AccessMode) MarshalJSON() ([]byte, error) {
	s := m.String()

	return json.Marshal(s)
}

// UnmarshalJSON un-marshals AccessMode from JSON
func (m *AccessMode) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	err := m.Set(s)
	if err != nil {
		return fmt.Errorf("invalid value '%s' for property 'mode' - %w", s, err)
	}

	return nil
}
