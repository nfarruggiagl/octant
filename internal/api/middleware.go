/*
Copyright (c) 2019 VMware, Inc. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	dashstrings "github.com/vmware/octant/internal/util/strings"
)

// rebindHandler is a middleware that will only accept the supplied hosts
func rebindHandler(acceptedHosts []string) mux.MiddlewareFunc {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var host string
			var err error
			if strings.Contains(r.Host, ":") {
				host, _, err = net.SplitHostPort(r.Host)
			} else {
				host = r.Host
			}

			if err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			if !dashstrings.Contains(host, acceptedHosts) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			h.ServeHTTP(w, r)
		})
	}
}
