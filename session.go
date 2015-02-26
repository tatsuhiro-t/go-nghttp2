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

// #cgo CFLAGS: -O2
// #cgo LDFLAGS: -lnghttp2
// #include <string.h>
// #include <nghttp2/nghttp2.h>
// #include "cnghttp2.h"
import "C"
import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

const (
	maxHeaderSize = 65536
)

// A session wraps around nghttp2 C interfaces.
type session struct {
	sc *serverConn
	ns *C.nghttp2_session
}

func newSession(sc *serverConn) *session {
	cbs := (*C.nghttp2_session_callbacks)(nil)
	C.nghttp2_session_callbacks_new(&cbs)
	defer C.nghttp2_session_callbacks_del(cbs)

	C.nghttp2_session_callbacks_set_on_begin_headers_callback(cbs, (C.nghttp2_on_begin_headers_callback)(C.on_begin_headers))
	C.nghttp2_session_callbacks_set_on_header_callback(cbs, (C.nghttp2_on_header_callback)(C.on_header))
	C.nghttp2_session_callbacks_set_on_frame_recv_callback(cbs, (C.nghttp2_on_frame_recv_callback)(C.on_frame_recv))
	C.nghttp2_session_callbacks_set_on_stream_close_callback(cbs, (C.nghttp2_on_stream_close_callback)(C.on_stream_close))
	C.nghttp2_session_callbacks_set_on_data_chunk_recv_callback(cbs, (C.nghttp2_on_data_chunk_recv_callback)(C.on_data_chunk_recv))

	opts := (*C.nghttp2_option)(nil)
	C.nghttp2_option_new(&opts)
	defer C.nghttp2_option_del(opts)

	C.nghttp2_option_set_recv_client_preface(opts, 1)
	C.nghttp2_option_set_no_auto_window_update(opts, 1)

	s := &session{
		sc: sc,
		ns: (*C.nghttp2_session)(nil),
	}

	C.nghttp2_session_server_new2(&s.ns, cbs, (unsafe.Pointer)(s), opts)

	return s
}

func (s *session) free() {
	C.nghttp2_session_del(s.ns)
	s.ns = nil
}

func (s *session) deserialize(p []byte, n int) error {
	rv := C.nghttp2_session_mem_recv(s.ns, (*C.uint8_t)(&p[0]), (C.size_t)(n))
	if (int)(rv) < 0 {
		return fmt.Errorf("nghttp2_session_mem_recv: %v\n", rv)
	}
	return nil
}

func (s *session) serialize() ([]byte, error) {
	var b *C.uint8_t
	n := C.nghttp2_session_mem_send(s.ns, &b)
	if n < 0 {
		return nil, fmt.Errorf("nghttp2_session_mem_send: %v", n)
	}
	if n == 0 {
		return nil, nil
	}
	return C.GoBytes((unsafe.Pointer)(b), (C.int)(n)), nil
}

func (s *session) wantReadWrite() bool {
	return C.nghttp2_session_want_read(s.ns) != 0 || C.nghttp2_session_want_write(s.ns) != 0
}

const (
	settingsMaxConcurrentStreams = (int32)(C.NGHTTP2_SETTINGS_MAX_CONCURRENT_STREAMS)
)

type settingsEntry struct {
	id    int32
	value uint32
}

func (s *session) submitSettings(ents []settingsEntry) error {
	cents := make([]C.nghttp2_settings_entry, len(ents))
	for i, v := range ents {
		cents[i].settings_id = (C.int32_t)(v.id)
		cents[i].value = (C.uint32_t)(v.value)
	}
	rv := C.nghttp2_submit_settings(s.ns, C.NGHTTP2_FLAG_NONE, &cents[0], (C.size_t)(len(cents)))
	if (int)(rv) != 0 {
		return fmt.Errorf("nghttp2_submit_settings: %v", rv)
	}
	return nil
}

func uint8Bytes(s string) (*C.uint8_t, C.size_t) {
	return (*C.uint8_t)((unsafe.Pointer)(C.CString(s))), (C.size_t)(len(s))
}

func (s *session) submitHeaders(st *stream, code int) error {
	nva := make([]C.nghttp2_nv, 1)
	nva[0].name, nva[0].namelen = uint8Bytes(":status")
	defer C.free((unsafe.Pointer)(nva[0].name))
	nva[0].value, nva[0].valuelen = uint8Bytes(strconv.Itoa(code))
	defer C.free((unsafe.Pointer)(nva[0].value))

	if rv := C.nghttp2_submit_headers(s.ns, C.NGHTTP2_FLAG_NONE, (C.int32_t)(st.id), nil, &nva[0], (C.size_t)(len(nva)), nil); rv != 0 {
		return fmt.Errorf("nghttp2_submit_rheader: %v", rv)
	}

	return nil
}

func (s *session) submitResponse(st *stream, eof bool) error {
	nvlen := 0
	rw := st.rw

	rw.snapHeader.Del(":status")
	rw.snapHeader.Del("Connection")
	rw.snapHeader.Del("Transfer-Encoding")

	if rw.snapHeader.Get("Date") == "" {
		rw.snapHeader.Add("Date", time.Now().UTC().Format(http.TimeFormat))
	}

	for _, vl := range rw.snapHeader {
		nvlen += len(vl)
	}
	nva := make([]C.nghttp2_nv, 1+nvlen)
	nva[0].name, nva[0].namelen = uint8Bytes(":status")
	defer C.free((unsafe.Pointer)(nva[0].name))
	nva[0].value, nva[0].valuelen = uint8Bytes(strconv.Itoa(rw.snapStatus))
	defer C.free((unsafe.Pointer)(nva[0].value))

	i := 1
	for k, vl := range rw.snapHeader {
		for _, v := range vl {
			nva[i].name, nva[i].namelen = uint8Bytes(k)
			defer C.free((unsafe.Pointer)(nva[i].name))
			nva[i].value, nva[i].valuelen = uint8Bytes(v)
			defer C.free((unsafe.Pointer)(nva[i].value))
			i++
		}
	}

	var prd *C.nghttp2_data_provider

	if eof {
		prd = nil
	} else {
		prd2 := make([]C.nghttp2_data_provider, 1)
		prd2[0].read_callback = (C.nghttp2_data_source_read_callback)(C.data_source_read)
		prd = &prd2[0]
	}
	if rv := C.nghttp2_submit_response(s.ns, (C.int32_t)(st.id), &nva[0], (C.size_t)(len(nva)), prd); rv != 0 {
		return fmt.Errorf("nghttp2_submit_response: %v", rv)
	}

	return nil
}

func (s *session) resumeData(st *stream) {
	C.nghttp2_session_resume_data(s.ns, (C.int32_t)(st.id))
}

const (
	noError       = (uint32)(C.NGHTTP2_NO_ERROR)
	protocolError = (uint32)(C.NGHTTP2_PROTOCOL_ERROR)
	internalError = (uint32)(C.NGHTTP2_INTERNAL_ERROR)
)

func (s *session) resetStream(st *stream) error {
	return s.resetStreamCode(st, protocolError)
}

func (s *session) resetStreamCode(st *stream, code uint32) error {
	rv := C.nghttp2_submit_rst_stream(s.ns, C.NGHTTP2_FLAG_NONE, (C.int32_t)(st.id), (C.uint32_t)(code))
	if rv != 0 {
		return fmt.Errorf("nghttp2_submit_rst_stream: %v", rv)
	}
	return nil
}

func (s *session) consume(st *stream, n int32) {
	C.nghttp2_session_consume(s.ns, (C.int32_t)(st.id), (C.size_t)(n))
}

//export onBeginHeaders
func onBeginHeaders(fr *C.nghttp2_frame, ptr unsafe.Pointer) C.int {
	if frameType(fr) != C.NGHTTP2_HEADERS {
		return 0
	}
	f := (*C.nghttp2_headers)((unsafe.Pointer)(fr))
	if f.cat != C.NGHTTP2_HCAT_REQUEST {
		return 0
	}

	s := (*session)(ptr)
	s.sc.openStream((int32)(f.hd.stream_id))

	return 0
}

func frameType(fr *C.nghttp2_frame) C.int {
	fhd := (*C.nghttp2_frame_hd)((unsafe.Pointer)(fr))
	return (C.int)(fhd._type)
}

//export onHeader
func onHeader(fr *C.nghttp2_frame, name *C.uint8_t, namelen C.size_t, value *C.uint8_t, valuelen C.size_t, flags C.uint8_t, ptr unsafe.Pointer) C.int {
	if frameType(fr) != C.NGHTTP2_HEADERS {
		return 0
	}
	f := (*C.nghttp2_headers)((unsafe.Pointer)(fr))
	if f.cat != C.NGHTTP2_HCAT_REQUEST {
		return 0
	}

	id := (int32)(f.hd.stream_id)
	s := (*session)(ptr)

	st, ok := s.sc.streams[id]
	if !ok {
		return 0
	}

	if st.rw != nil {
		return 0
	}

	k := C.GoStringN((*C.char)((unsafe.Pointer)(name)), (C.int)(namelen))
	v := C.GoStringN((*C.char)((unsafe.Pointer)(value)), (C.int)(valuelen))
	v = strings.TrimSpace(v)

	if k[0] == ':' {
		switch k {
		case ":authority":
			st.authority = v
		case ":method":
			st.method = v
		case ":path":
			st.path = v
		case ":scheme":
			st.scheme = v
		}
	} else {
		st.header.Add(k, v)
	}

	st.headerSize += len(k) + len(v)
	if st.headerSize > maxHeaderSize {
		s.sc.handleError(st, 431)
		return 0
	}

	return 0
}

//export onFrameRecv
func onFrameRecv(fr *C.nghttp2_frame, ptr unsafe.Pointer) C.int {
	s := (*session)(ptr)
	ft := frameType(fr)
	switch ft {
	case C.NGHTTP2_DATA:
		f := (*C.nghttp2_data)((unsafe.Pointer)(fr))
		if (f.hd.flags & C.NGHTTP2_FLAG_END_STREAM) != 0 {
			id := (int32)(f.hd.stream_id)
			st, ok := s.sc.streams[id]
			if !ok {
				return 0
			}
			s.sc.handleUpload(st, nil)
		}
	case C.NGHTTP2_HEADERS:
		f := (*C.nghttp2_headers)((unsafe.Pointer)(fr))
		if f.cat != C.NGHTTP2_HCAT_REQUEST {
			return 0
		}
		id := (int32)(f.hd.stream_id)
		st, ok := s.sc.streams[id]
		if !ok {
			return 0
		}

		es := f.hd.flags&C.NGHTTP2_FLAG_END_STREAM != 0

		if err := s.sc.headerReadDone(st, es); err != nil {
			return C.NGHTTP2_ERR_CALLBACK_FAILURE
		}
		if es {
			s.sc.handleUpload(st, nil)
		}
	}

	return 0
}

//export onStreamClose
func onStreamClose(id C.int32_t, ec C.uint32_t, ptr unsafe.Pointer) C.int {
	s := (*session)(ptr)
	st, ok := s.sc.streams[(int32)(id)]
	if !ok {
		return 0
	}
	s.sc.closeStream(st, (uint32)(ec))
	return 0
}

//export dataSourceRead
func dataSourceRead(cid C.int32_t, buf *C.uint8_t, buflen C.size_t, dflags *C.uint32_t, ptr unsafe.Pointer) C.ssize_t {
	s := (*session)(ptr)
	id := (int32)(cid)
	st, ok := s.sc.streams[id]
	if !ok {
		return C.NGHTTP2_ERR_DEFERRED
	}
	rw := st.rw
	n := (int)(buflen)

	if n > len(rw.p) {
		n = len(rw.p)
	}

	if n > 0 {
		C.memcpy((unsafe.Pointer)(buf), (unsafe.Pointer)(&rw.p[0]), (C.size_t)(n))
		rw.p = rw.p[n:]
		if len(rw.p) == 0 {
			rw.dataDoneCh <- struct{}{}
		}
	}

	if rw.es {
		*dflags |= C.NGHTTP2_DATA_FLAG_EOF
		if req := rw.req; req != nil {
			rb := req.Body.(*requestBody)
			if !rb.endStream() {
				if err := s.resetStreamCode(st, noError); err != nil {
					return C.NGHTTP2_ERR_CALLBACK_FAILURE
				}
			}
		}
	} else if n == 0 {
		return C.NGHTTP2_ERR_DEFERRED
	}
	return (C.ssize_t)(n)
}

//export dataChunkRecv
func dataChunkRecv(cid C.int32_t, buf *C.uint8_t, buflen C.size_t, ptr unsafe.Pointer) C.int {
	s := (*session)(ptr)
	id := (int32)(cid)
	st, ok := s.sc.streams[id]
	if !ok {
		return 0
	}
	p := C.GoBytes((unsafe.Pointer)(buf), (C.int)(buflen))
	s.sc.handleUpload(st, p)
	return 0
}
