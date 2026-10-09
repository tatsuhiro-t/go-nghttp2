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
// #cgo LDFLAGS: -lnghttp2v2
// #include <string.h>
// #include "cnghttp2.h"
import "C"
import (
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"runtime/cgo"
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
	ns *C.nghttp2_conn
	h  cgo.Handle
}

func newSession(sc *serverConn) *session {
	var callbacks C.nghttp2_callbacks

	callbacks.rand = (C.nghttp2_rand)(C.rand_bytes)
	callbacks.begin_headers = (C.nghttp2_begin_fields)(C.begin_headers)
	callbacks.recv_header = (C.nghttp2_recv_field)(C.recv_header)
	callbacks.end_headers = (C.nghttp2_end_fields)(C.end_headers)
	callbacks.recv_data = (C.nghttp2_recv_data)(C.recv_data)
	callbacks.remote_end_stream = (C.nghttp2_end_stream)(C.remote_end_stream)
	callbacks.stream_close = (C.nghttp2_stream_close)(C.stream_close)
	callbacks.write_stream_data_offset = (C.nghttp2_write_stream_data_offset)(C.write_stream_data_offset)

	var settings C.nghttp2_settings

	C.nghttp2_settings_default(&settings)

	if os.Getenv("GO_NGHTTP2_DEBUG_LOG") == "1" {
		settings.log_write = (C.nghttp2_log_write)(C.log_write)
	}

	settings.initial_ts = timestamp()
	settings.max_concurrent_streams_remote = 100

	s := &session{
		sc: sc,
		ns: (*C.nghttp2_conn)(nil),
	}

	h := cgo.NewHandle(s)
	s.h = h

	C.nghttp2_conn_server_new(&s.ns, &callbacks, &settings, nil, (unsafe.Pointer)(uintptr(h)))

	return s
}

func (s *session) free() {
	C.nghttp2_conn_del(s.ns)
	s.h.Delete()
	s.ns = nil
}

func timestamp() C.nghttp2_tstamp {
	return (C.nghttp2_tstamp)(time.Now().UnixNano())
}

func (s *session) deserialize(p []byte) error {
	cp := unsafe.Pointer(unsafe.SliceData(p))

	rv := C.nghttp2_conn_read(s.ns, (*C.uint8_t)(cp), (C.size_t)(len(p)), timestamp())
	if rv != 0 {
		return fmt.Errorf("nghttp2_conn_read: %v", rv)
	}

	return nil
}

func (s *session) serialize(dest []byte) (int, error) {
	cp := unsafe.Pointer(unsafe.SliceData(dest))

	n := C.nghttp2_conn_write(s.ns, (*C.uint8_t)(cp), (C.size_t)(len(dest)), timestamp())
	if n < 0 {
		return 0, fmt.Errorf("nghttp2_conn_write: %v", n)
	}
	if n == 0 {
		return 0, nil
	}
	return int(n), nil
}

func uint8Bytes(s string) (*C.uint8_t, C.size_t) {
	return (*C.uint8_t)((unsafe.Pointer)(C.CString(s))), (C.size_t)(len(s))
}

func (s *session) submitInfo(st *stream, code int) error {
	nva := make([]C.nghttp2_nv, 1)
	nva[0].name, nva[0].namelen = uint8Bytes(":status")
	defer C.free((unsafe.Pointer)(nva[0].name))
	nva[0].value, nva[0].valuelen = uint8Bytes(strconv.Itoa(code))
	defer C.free((unsafe.Pointer)(nva[0].value))

	if rv := C.nghttp2_conn_submit_info(s.ns, (C.int64_t)(st.id), &nva[0], (C.size_t)(len(nva))); rv != 0 {
		return fmt.Errorf("nghttp2_conn_submit_info: %v", rv)
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

	var pdr *C.nghttp2_data_reader

	if !eof {
		var dr C.nghttp2_data_reader

		dr.read_data = (C.nghttp2_read_data)(C.read_data)
		pdr = &dr
	}
	if rv := C.nghttp2_conn_submit_response(s.ns, (C.int64_t)(st.id), &nva[0], (C.size_t)(len(nva)), pdr); rv != 0 {
		return fmt.Errorf("nghttp2_conn_submit_response: %v", rv)
	}

	return nil
}

func (s *session) resumeStream(st *stream) {
	C.nghttp2_conn_resume_stream(s.ns, (C.int64_t)(st.id))
}

const (
	noError       = (uint32)(C.NGHTTP2_NO_ERROR)
	protocolError = (uint32)(C.NGHTTP2_PROTOCOL_ERROR)
	internalError = (uint32)(C.NGHTTP2_INTERNAL_ERROR)
)

func (s *session) resetStream(st *stream) {
	s.resetStreamCode(st, protocolError)
}

func (s *session) resetStreamCode(st *stream, code uint32) {
	C.nghttp2_conn_shutdown_stream(s.ns, (C.uint32_t)(0x00), (C.int64_t)(st.id), (C.uint32_t)(code))
}

func (s *session) consume(st *stream, n int32) {
	C.nghttp2_conn_extend_max_stream_offset(s.ns, (C.int64_t)(st.id), (C.size_t)(n))
	C.nghttp2_conn_extend_max_offset(s.ns, (C.size_t)(n))
}

func session_from_ptr(ptr unsafe.Pointer) *session {
	h := cgo.Handle(uintptr(ptr))
	return h.Value().(*session)
}

//export randBytes
func randBytes(data *C.uint8_t, datalen C.size_t) {
	s := unsafe.Slice((*byte)(unsafe.Pointer(data)), int(datalen))
	rand.Read(s)
}

//export beginHeaders
func beginHeaders(sid C.int64_t, ptr unsafe.Pointer) C.int {
	s := session_from_ptr(ptr)
	s.sc.openStream((int64)(sid))
	return 0
}

//export recvHeader
func recvHeader(sid C.int64_t, name *C.uint8_t, namelen C.size_t, value *C.uint8_t, valuelen C.size_t, _ C.uint8_t, ptr unsafe.Pointer) C.int {
	s := session_from_ptr(ptr)
	id := (int64)(sid)

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

//export endHeaders
func endHeaders(sid C.int64_t, fin C.int, ptr unsafe.Pointer) C.int {
	s := session_from_ptr(ptr)
	id := (int64)(sid)
	st, ok := s.sc.streams[id]
	if !ok {
		return 0
	}

	es := fin != 0

	if err := s.sc.headerReadDone(st, es); err != nil {
		return C.NGHTTP2_ERR_CALLBACK_FAILURE
	}
	if es {
		s.sc.handleUpload(st, nil)
	}

	return 0
}

//export recvData
func recvData(sid C.int64_t, data *C.uint8_t, datalen C.size_t, ptr unsafe.Pointer) C.int {
	s := session_from_ptr(ptr)
	id := (int64)(sid)
	st, ok := s.sc.streams[id]
	if !ok {
		return 0
	}
	s.sc.handleUpload(st,
		unsafe.Slice((*byte)(unsafe.Pointer(data)), int(datalen)))
	return 0
}

//export remoteEndStream
func remoteEndStream(sid C.int64_t, ptr unsafe.Pointer) C.int {
	s := session_from_ptr(ptr)
	id := (int64)(sid)
	st, ok := s.sc.streams[id]
	if !ok {
		return 0
	}
	s.sc.handleUpload(st, nil)
	return 0
}

//export streamClose
func streamClose(sid C.int64_t, _ C.uint32_t, ptr unsafe.Pointer) C.int {
	s := session_from_ptr(ptr)
	st, ok := s.sc.streams[(int64)(sid)]
	if !ok {
		return 0
	}
	s.sc.closeStream(st)
	return 0
}

//export writeStreamDataOffset
func writeStreamDataOffset(sid C.int64_t, datalen C.size_t, ptr unsafe.Pointer) C.int {
	s := session_from_ptr(ptr)
	id := (int64)(sid)
	st, ok := s.sc.streams[id]
	if !ok {
		return 0
	}

	rw := st.rw

	rw.pinner.size -= int(datalen)
	if rw.pinner.size == 0 {
		rw.pinner.pin.Unpin()
		rw.dataDoneCh <- struct{}{}
	}

	return 0
}

//export readData
func readData(sid C.int64_t, vec *C.nghttp2_vec, veccnt C.size_t, pflags *C.uint32_t, ptr unsafe.Pointer) C.nghttp2_ssize {
	s := session_from_ptr(ptr)
	id := (int64)(sid)
	st, ok := s.sc.streams[id]
	if !ok {
		return C.NGHTTP2_ERR_WOULDBLOCK
	}

	rw := st.rw

	if rw.es {
		*pflags |= C.NGHTTP2_READ_DATA_FLAG_EOF

		if req := rw.req; req != nil {
			rb := req.Body.(*requestBody)
			if !rb.endStream() {
				s.resetStreamCode(st, noError)
			}
		}

		if len(rw.p) == 0 {
			return 0
		}
	} else if len(rw.p) == 0 {
		return C.NGHTTP2_ERR_WOULDBLOCK
	}

	dataPtr := unsafe.SliceData(rw.p)

	rw.pinner.pin.Pin(dataPtr)
	rw.pinner.size = len(rw.p)

	vec.base = (*C.uint8_t)(unsafe.Pointer(dataPtr))
	vec.len = (C.size_t)(len(rw.p))

	rw.p = nil

	return 1
}

//export logWrite
func logWrite(msg *C.char, msglen C.size_t) {
	p := unsafe.String((*byte)(unsafe.Pointer(msg)), int(msglen))
	fmt.Fprintln(os.Stderr, p)
}
