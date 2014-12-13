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
#include <nghttp2/nghttp2.h>
#include "cnghttp2.h"
#include "_cgo_export.h"

int on_begin_headers(nghttp2_session *session, const nghttp2_frame *frame,
                     void *user_data)
{
  int rv;
  rv = onBeginHeaders((nghttp2_frame *)frame, user_data);
  return rv;
}

int on_header(nghttp2_session *session, const nghttp2_frame *frame,
              const uint8_t *name, size_t namelen, const uint8_t *value,
              size_t valuelen, uint8_t flags, void *user_data)
{
  int rv;
  rv = onHeader((nghttp2_frame *)frame, (uint8_t *)name, namelen,
                (uint8_t *)value, valuelen, flags, user_data);
  return rv;
}

int on_frame_recv(nghttp2_session *session, const nghttp2_frame *frame,
                  void *user_data) {
  int rv;
  rv = onFrameRecv((nghttp2_frame *)frame, user_data);
  return rv;
}

int on_stream_close(nghttp2_session *session, int32_t stream_id,
                    uint32_t error_code, void *user_data)
{
  int rv;
  rv = onStreamClose(stream_id, error_code, user_data);
  return rv;
}

ssize_t data_source_read(nghttp2_session *session, int32_t stream_id,
                         uint8_t *buf, size_t length, uint32_t *data_flags,
                         nghttp2_data_source *source, void *user_data) {
  ssize_t rv;
  rv = dataSourceRead(stream_id, buf, length, data_flags, user_data);
  return rv;
}

int on_data_chunk_recv(nghttp2_session *session, uint8_t flags,
                       int32_t stream_id, const uint8_t *data, size_t len,
                       void *user_data) {
  int rv;
  rv = dataChunkRecv(stream_id, (uint8_t *)data, len, user_data);
  return rv;
}
