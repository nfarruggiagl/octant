/*
Copyright (c) 2019 VMware, Inc. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package event

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/vmware/octant/internal/octant"
)

type statusResponse struct {
	Message   string `json:"message,omitempty"`
	State     string `json:"state,omitempty"`
	Timestamp int64  `json:"timeStamp,omitempty"`
	TimeDiff  string `json:"timeDiff,omitempty"`
}

// StatusGenerator generates status events.
type StatusGenerator struct {
}

var _ octant.Generator = (*StatusGenerator)(nil)

// Event generates status events
func (g *StatusGenerator) Event(ctx context.Context) (octant.Event, error) {
	nr := &statusResponse{
		Message:   fmt.Sprintf("Test message: %s", time.Now()),
		State:     "error",
		Timestamp: time.Now().Unix(),
		TimeDiff:  "less than 1 minute",
	}
	data, err := json.Marshal(nr)
	if err != nil {
		return octant.Event{}, errors.New("unable to marshal status")
	}

	return octant.Event{
		Type: octant.EventTypeNamespaces,
		Data: data,
	}, nil
}

// ScheduleDelay returns how long to delay before running this generator again.
func (StatusGenerator) ScheduleDelay() time.Duration {
	return DefaultScheduleDelay
}

// Name returns the generator's name.
func (StatusGenerator) Name() string {
	return "errors"
}
