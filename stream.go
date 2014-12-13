// go-nghttp2
//
// Copyright (c) 2014 Tatsuhiro Tsujikawa
//
// Permission is hereby granted, free of charge, to any person obtaining
// a copy of this software and associated documentation files (the
// "Software"), to deal in the Software without restriction, including
// without limitation the rights to use, copy, modify, merge, publish,
// distribute, sublicense, and/or sell copies of the Software, and to
// permit persons to whom the Software is furnished to do so, subject to
// the following conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
// LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
// OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
// WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package nghttp2

import (
	"io"
	"net/http"
	"strconv"
	"sync"
)

type stream struct {
	rw            *responseWriter
	id            int32       // stream ID
	header        http.Header // request header fields
	mustRegHeader bool        // next incoming header must be regular header, not pseudo header
	headerSize    int         // limit to the sum of header field name/value size in total
}

type requestBody struct {
	rw *responseWriter

	closed bool // handler closed request body

	mu sync.Mutex // guards following
	c  sync.Cond
	p  []byte // unread request body
	es bool   // END_STREAM seen, i.e., half-closed (remote)
}

func (rb *requestBody) Read(p []byte) (int, error) {
	if rb.closed {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	rb.c.L.Lock()
	defer rb.c.L.Unlock()
	if len(rb.p) == 0 && !rb.es {
		rb.c.Wait()
	}
	if len(rb.p) == 0 && rb.es {
		return 0, io.EOF
	}
	n := copy(p, rb.p)
	rb.p = rb.p[n:]
	rw := rb.rw
	rw.sc.writeReqCh <- &writeReq{
		t:  writeReqConsumed,
		rw: rw,
		n:  (int32)(n),
	}

	return n, nil
}

func (rb *requestBody) Close() error {
	rb.closed = true
	return nil
}

func (rb *requestBody) write(p []byte) (int, error) {
	if rb.closed {
		return len(p), nil
	}

	rb.c.L.Lock()
	defer rb.c.L.Unlock()
	if len(p) == 0 {
		rb.es = true
	} else {
		rb.p = append(rb.p, p...)
	}
	rb.c.Signal()
	return len(p), nil
}

func (rb *requestBody) endStream() bool {
	rb.c.L.Lock()
	defer rb.c.L.Unlock()
	return rb.es
}

type responseWriter struct {
	// read only
	sc         *serverConn
	st         *stream
	req        *http.Request
	es         bool        // write is done, i.e., half-closed (local)
	p          []byte      // chunk of bytes to write
	snapStatus int         // status code sent in response
	snapHeader http.Header // header fields actually sent in response

	// mutated by handler goroutine
	eof           bool        // no more write is done from handler
	headerSent    bool        // response header has been sent
	status        int         // status code set by application
	handlerHeader http.Header // header fields set by application
	handlerDone   bool        // handler finished
	contentLength int64       // explicitly-declared Content-Length; or -1
	written       int64       // number of bytes written in body

	mu           sync.Mutex // guards the following
	c            sync.Cond
	disconnected bool // connection is terminated
}

func (rw *responseWriter) Header() http.Header {
	return rw.handlerHeader
}

func (rw *responseWriter) Write(p []byte) (n int, err error) {
	// TODO writing closed channel causes panic
	defer func() {
		if r := recover(); r != nil {
			n = 0
			err = http.ErrWriteAfterFlush
		}
	}()
	if !rw.headerSent {
		rw.headerSent = true
		if rw.status == 0 {
			rw.WriteHeader(http.StatusOK)
		}

		status := rw.status
		header := cloneHeader(rw.handlerHeader)

		if cl := header.Get("Content-Length"); cl != "" {
			v, err := strconv.ParseInt(cl, 10, 64)
			if err == nil && v >= 0 {
				rw.contentLength = v
			} else {
				header.Del("Content-Length")
			}
		}

		rw.eof = (rw.req != nil && rw.req.Method == "HEAD") || rw.contentLength == 0 || status == 304 || status == 204

		rw.sc.writeReqCh <- &writeReq{
			t:      writeReqResponse,
			rw:     rw,
			es:     rw.eof,
			status: status,
			header: header,
		}
	}

	if rw.eof {
		return len(p), nil
	}

	rw.written += int64(len(p))
	if rw.contentLength != -1 && rw.written > rw.contentLength {
		return 0, http.ErrContentLength
	}

	rw.eof = rw.handlerDone && len(p) == 0

	rw.sc.writeReqCh <- &writeReq{
		t:  writeReqData,
		rw: rw,
		p:  p,
		es: rw.eof,
	}

	// if eof is seen, we don't wait for signal
	if rw.eof {
		return len(p), nil
	}

	rw.c.L.Lock()
	defer rw.c.L.Unlock()
	rw.c.Wait()

	if rw.disconnected {
		rw.eof = true
	}

	return len(p), nil
}

func (rw *responseWriter) resetStream() {
	// TODO writing closed channel causes panic
	defer func() {
		recover()
	}()
	rw.sc.writeReqCh <- &writeReq{
		t:       writeReqRstStream,
		rw:      rw,
		errCode: internalError,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
}

func (rw *responseWriter) finishRequest() {
	rw.handlerDone = true
	rw.Write(nil)
}

func cloneHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, v := range src {
		nv := make([]string, len(v))
		copy(nv, v)
		dst[k] = nv
	}
	return dst
}
