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
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	H2Proto = "h2-14"
)

// ConfigureServer adds HTTP/2 support to http.Server.
func ConfigureServer(hs *http.Server, conf *Server) {
	// Mostof the code in this function was copied from
	// https://github.com/bradfitz/http2/blob/master/server.go
	if conf == nil {
		conf = new(Server)
	}
	if hs.TLSConfig == nil {
		hs.TLSConfig = new(tls.Config)
	}

	hs.TLSConfig.NextProtos = append(hs.TLSConfig.NextProtos, H2Proto)

	if hs.TLSNextProto == nil {
		hs.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){}
	}
	hs.TLSNextProto[H2Proto] = func(hs *http.Server, c *tls.Conn, h http.Handler) {
		conf.handleConn(hs, c, h)
	}
}

type Server struct {
}

func (srv *Server) handleConn(hs *http.Server, rwc net.Conn, h http.Handler) {
	br := bufio.NewReader(rwc)
	bw := bufio.NewWriterSize(rwc, 4096)
	buf := bufio.NewReadWriter(br, bw)

	sc := &serverConn{
		rwc:        rwc,
		handler:    h,
		streams:    make(map[int32]*stream),
		remoteAddr: rwc.RemoteAddr().String(),
		buf:        buf,
		inBuf:      make([]byte, 4096),
		readCh:     make(chan int),
		readDoneCh: make(chan struct{}),
		writeReqCh: make(chan *writeReq, 200),
	}
	sc.s = newSession(sc)

	if tc, ok := rwc.(*tls.Conn); ok {
		sc.tlsState = new(tls.ConnectionState)
		*sc.tlsState = tc.ConnectionState()
	}
	sc.serve()
}

const (
	writeReqResponse  = iota // request to write final HEADERS
	writeReqData             // request to write one or more DATA
	writeReqRstStream        // request to write RST_STREAM
	writeReqConsumed         // indicates that some uploaded bytes are consumed
)

type writeReq struct {
	t  int             // type of request; writeReqResponse or writeReqData
	rw *responseWriter // responseWriter this request refers to
	es bool            // no more read/write HEADER or DATA is expected

	status int         // status code if t == writeReqResponse
	header http.Header // header fields if t == writeReqResponse

	p []byte // data chunk if t == writeReqData

	errCode uint32 // error code if t == writeReqRstStream

	n int32 // the number of consumed bytes if t == writeReqConsumed
}

type serverConn struct {
	rwc        net.Conn             // i/o connection
	remoteAddr string               // network address of remote side
	tlsState   *tls.ConnectionState // or nil when not using TLS
	handler    http.Handler         // to handle request
	streams    map[int32]*stream    // to store HTTP/2 streams

	buf   *bufio.ReadWriter // buffered(rwc, rwc)
	inBuf []byte            // input buffer

	s *session // session wrapper to nghttp2 C interface

	readCh     chan int
	readDoneCh chan struct{}
	writeReqCh chan *writeReq

	wg sync.WaitGroup // to wait for all handler goroutines finish
}

func (sc *serverConn) serve() {
	defer func() {
		sc.s.free()
		sc.rwc.Close()
		close(sc.readDoneCh)
		for _, st := range sc.streams {
			if st.rw == nil {
				continue
			}
			close(st.rw.dataDoneCh)
		}
		// wait for handler finished before closing
		// sc.writeReqCh which handler may write into.
		sc.wg.Wait()
		close(sc.writeReqCh)
	}()

	if err := sc.s.submitSettings([]settingsEntry{{SETTINGS_MAX_CONCURRENT_STREAMS, 100}}); err != nil {
		return
	}

	go sc.doRead()

	for {
		if err := sc.doWrite(); err != nil {
			return
		}

		select {
		case n, ok := <-sc.readCh:
			if !ok {
				return
			}
			if err := sc.handleInput(n); err != nil {
				return
			}
			sc.readDoneCh <- struct{}{}
		case wreq, ok := <-sc.writeReqCh:
		Loop:
			for {
				if !ok {
					return
				}
				if err := sc.handleWriteReq(wreq); err != nil {
					return
				}
				select {
				case wreq, ok = <-sc.writeReqCh:
					break
				default:
					break Loop
				}
			}
		}
	}
}

func (sc *serverConn) handleWriteReq(wreq *writeReq) error {
	rw := wreq.rw
	if wreq.es {
		rw.es = true
	}
	switch wreq.t {
	case writeReqResponse:
		rw.snapStatus = wreq.status
		rw.snapHeader = wreq.header
		if err := sc.s.submitResponse(rw.st, rw.es); err != nil {
			return fmt.Errorf("sc.s.submitResponse: %v\n", err)
		}
	case writeReqData:
		rw.p = wreq.p
		sc.s.resumeData(rw.st)
	case writeReqRstStream:
		sc.s.resetStreamCode(rw.st, wreq.errCode)
	case writeReqConsumed:
		sc.s.consume(rw.st, wreq.n)
	}
	return nil
}

func (sc *serverConn) doRead() {
	for {
		n, err := sc.buf.Read(sc.inBuf)
		if err != nil {
			close(sc.readCh)
			return
		}

		select {
		case sc.readCh <- n:
		case <-sc.readDoneCh:
			// serve() already existed
			close(sc.readCh)
			return
		}

		if _, ok := <-sc.readDoneCh; !ok {
			close(sc.readCh)
			return
		}
	}
}

func (sc *serverConn) handleInput(n int) error {
	if err := sc.s.deserialize(sc.inBuf, n); err != nil {
		return err
	}
	return nil
}

func (sc *serverConn) doWrite() error {
	for {
		p, err := sc.s.serialize()
		if err != nil {
			return err
		}
		if p == nil {
			break
		}
		if _, err := sc.buf.Write(p); err != nil {
			return err
		}
	}
	sc.buf.Flush()
	if !sc.s.wantReadWrite() {
		return fmt.Errorf("no more/read write for this session")
	}
	return nil
}

func (sc *serverConn) runHandler(rw *responseWriter, req *http.Request) {
	defer sc.wg.Done()
	defer func() {
		if err := recover(); err != nil {
			rw.resetStream()
		}
	}()
	defer rw.finishRequest()
	sc.handler.ServeHTTP(rw, req)
}

func (sc *serverConn) openStream(id int32) {
	st := &stream{
		id:     id,
		header: make(http.Header),
	}
	sc.streams[id] = st
}

func (sc *serverConn) closeStream(st *stream, errCode uint32) {
	if st.rw != nil {
		close(st.rw.dataDoneCh)
	}
	delete(sc.streams, st.id)
}

func (sc *serverConn) headerReadDone(st *stream) error {
	if st.rw != nil {
		// just return if response is already committed
		return nil
	}

	host := st.authority
	if host == "" {
		host = st.header.Get("host")
	}

	justAuthority := false
	rawurl := st.path

	if st.method == "CONNECT" {
		if st.authority == "" || st.scheme != "" || st.path != "" {
			if err := sc.s.resetStream(st); err != nil {
				return fmt.Errorf("sc.s.resetStream(st) failed")
			}
			return nil
		}
		justAuthority = true
		rawurl = "http://" + rawurl
	} else {
		if host == "" || st.path == "" || st.method == "" || st.scheme == "" {
			if err := sc.s.resetStream(st); err != nil {
				return fmt.Errorf("sc.s.resetStream(st) failed")
			}
			return nil
		}
	}

	url, err := url.ParseRequestURI(rawurl)
	if err != nil {
		if err := sc.s.resetStream(st); err != nil {
			return fmt.Errorf("sc.s.resetStream(st) failed")
		}
		return nil
	}

	// do the same way in net/http/server.go
	if justAuthority {
		url.Scheme = ""
	}

	if st.header.Get("host") == "" {
		st.header.Set("Host", host)
	}

	// cookie header field may be split into multiple fields.
	// Concatenate them into one.
	var cookieBuf bytes.Buffer
	if cookies, ok := st.header["Cookie"]; ok {
		if len(cookies) > 1 {
			for _, c := range cookies {
				cookieBuf.WriteString(c)
				cookieBuf.WriteString("; ")
			}
			cookieBuf.Truncate(cookieBuf.Len() - 2)
			st.header.Set("Cookie", cookieBuf.String())
		}
	}

	rb := &requestBody{}
	rb.c.L = &rb.mu

	req := &http.Request{
		Method:     st.method,
		URL:        url,
		RemoteAddr: sc.remoteAddr,
		Header:     st.header,
		RequestURI: st.path,
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		ProtoMinor: 0,
		TLS:        sc.tlsState,
		Host:       host,
		Body:       rb,
	}

	rw := &responseWriter{
		sc:            sc,
		st:            st,
		req:           req,
		handlerHeader: make(http.Header),
		contentLength: -1,
		// Create at least 1 space so that write is not
		// blocked
		dataDoneCh: make(chan struct{}, 1),
	}

	st.rw = rw
	rb.rw = rw

	if expect := st.header.Get("Expect"); expect != "" {
		expect = strings.ToLower(expect)
		if strings.Contains(expect, "100-continue") {
			st.header.Del("Expect")
			if err := sc.s.submitHeaders(st, 100); err != nil {
				return err
			}
		}
	}

	sc.wg.Add(1)
	go sc.runHandler(rw, req)

	return nil
}

// handleError returns error page.  We don't create http.Request
// object in this case.
func (sc *serverConn) handleError(st *stream, code int) {
	rw := &responseWriter{
		sc:            sc,
		st:            st,
		handlerHeader: make(http.Header),
		contentLength: -1,
	}

	st.rw = rw

	sc.wg.Add(1)
	go func() {
		defer sc.wg.Done()
		defer rw.finishRequest()
		http.Error(rw, fmt.Sprintf("%v %v", code, http.StatusText(code)), code)
	}()
}

func (sc *serverConn) handleUpload(st *stream, p []byte) {
	if st.rw == nil {
		return
	}
	req := st.rw.req
	if req == nil {
		return
	}
	rb := req.Body.(*requestBody)
	rb.write(p)
}
