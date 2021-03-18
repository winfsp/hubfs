/*
 * httputil.go
 *
 * Copyright 2021 Bill Zissimopoulos
 */
/*
 * This file is part of Hubfs.
 *
 * It is licensed under the MIT license. The full license text can be found
 * in the License.txt file at the root of this project.
 */

package httputil

import (
	"net/http"
	"time"

	"github.com/billziss-gh/golib/retry"
)

var (
	DefaultRetryCount = 10
	DefaultSleep      = time.Second
	DefaultMaxSleep   = time.Second * 30
	DefaultClient     = &http.Client{
		Transport: &transport{
			RoundTripper: http.DefaultTransport,
		},
	}
)

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
