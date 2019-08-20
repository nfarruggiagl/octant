/*
Copyright (c) 2019 VMware, Inc. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"encoding/json"
	"net/http"

	"github.com/vmware/octant/internal/log"
	"github.com/vmware/octant/internal/mime"
)

type status struct {
	logger log.Logger
}

func newStatus(logger log.Logger) *status {
	return &status{
		logger: logger,
	}
}

type tabStream struct {
	DisplayName string `json:"displayName,omitempty"`
	StreamName  string `json:"streamName,omitempty"`
}

func (s *status) tabs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", mime.JSONContentType)

	nr := &[]tabStream{
		{DisplayName: "Errors", StreamName: "errors"},
	}

	if err := json.NewEncoder(w).Encode(nr); err != nil {
		s.logger.Errorf("encoding namespaces: %v", err)
	}
}
