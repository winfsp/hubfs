/*
 * httputil.go
 *
 * Copyright 2021-2022 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * You can redistribute it and/or modify it under the terms of the GNU
 * Affero General Public License version 3 as published by the Free
 * Software Foundation.
 */

package httputil

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/billziss-gh/golib/retry"
)

var (
	DefaultRetryCount = 10
	DefaultSleep      = time.Second
	DefaultMaxSleep   = time.Second * 30
	DefaultClient     *http.Client
	DefaultTransport  *http.Transport
)

func init() {
	DefaultTransport = http.DefaultTransport.(*http.Transport).Clone()
	if nil == DefaultTransport.TLSClientConfig {
		DefaultTransport.TLSClientConfig = &tls.Config{}
	}
	DefaultClient = &http.Client{
		Transport: &transport{
			RoundTripper: DefaultTransport,
		},
	}
}

type transport struct {
	http.RoundTripper
}

func (t *transport) RoundTrip(req *http.Request) (rsp *http.Response, err error) {
	retry.Retry(
		retry.Count(DefaultRetryCount),
		retry.Backoff(DefaultSleep, DefaultMaxSleep),
		func(i int) bool {

			rsp, err = t.RoundTripper.RoundTrip(req)

			// retry on connection errors without body
			if nil != err {
				return nil == req.Body
			}

			// retry on HTTP 429, 503, 509
			switch rsp.StatusCode {
			case 429, 503, 509:
				rsp.Body.Close()
				return true
			}

			return false
		})

	return
}
