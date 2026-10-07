/*
 * go-nghttp2
 *
 * Copyright (c) 2014 Tatsuhiro Tsujikawa
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be
 * included in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
 * NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
 * LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
 * OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
 * WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */
#include "cnghttp2.h"
#include "_cgo_export.h"

void rand_bytes(uint8_t *dest, size_t destlen) { randBytes(dest, destlen); }

int begin_headers(nghttp2_conn *conn, int64_t stream_id, void *conn_user_data,
                  void *stream_user_data) {
  return beginHeaders(stream_id, conn_user_data);
}

int recv_header(nghttp2_conn *conn, int64_t stream_id, int32_t token,
                nghttp2_rcbuf *name, nghttp2_rcbuf *value, uint8_t flags,
                void *conn_user_data, void *stream_user_data) {
   nghttp2_vec namebuf = nghttp2_rcbuf_get_buf(name);
   nghttp2_vec valuebuf = nghttp2_rcbuf_get_buf(value);
   return recvHeader(stream_id, namebuf.base, namebuf.len, valuebuf.base,
                     valuebuf.len, flags, conn_user_data);
}

int end_headers(nghttp2_conn *conn, int64_t stream_id, int fin,
                void *conn_user_data, void *stream_user_data) {
  return endHeaders(stream_id, fin, conn_user_data);
}

int recv_data(nghttp2_conn *conn, int64_t stream_id, const uint8_t *data,
              size_t datalen, void *conn_user_data, void *stream_user_data) {
  return recvData(stream_id, (uint8_t *)data, datalen, conn_user_data);
}

int remote_end_stream(nghttp2_conn *conn, int64_t stream_id,
                      void *conn_user_data, void *stream_user_data) {
  return remoteEndStream(stream_id, conn_user_data);
}

int stream_close(nghttp2_conn *conn, uint32_t flags, int64_t stream_id,
                 uint32_t error_code, void *conn_user_data,
                 void *stream_user_data) {
  if (!(flags & NGHTTP2_STREAM_CLOSE_FLAG_ERROR_CODE_SET)) {
    error_code = NGHTTP2_NO_ERROR;
  }

  return streamClose(stream_id, error_code, conn_user_data);
}

int write_stream_data_offset(nghttp2_conn *conn, int64_t stream_id,
                             uint64_t offset, size_t len, void *conn_user_data,
                             void *stream_user_data) {
  return writeStreamDataOffset(stream_id, len, conn_user_data);
}

nghttp2_ssize read_data(nghttp2_conn *conn, int64_t stream_id, nghttp2_vec *vec,
                        size_t veccnt, uint32_t *pflags, void *conn_user_data,
                        void *stream_user_data) {
  return readData(stream_id, vec, veccnt, pflags, conn_user_data);
}

void log_write(void *user_data, char *msg, size_t len) {
  logWrite(msg, len);
}
