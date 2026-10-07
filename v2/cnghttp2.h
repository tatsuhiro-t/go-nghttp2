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
#ifndef CNGHTTP2_H
#define CNGHTTP2_H

#include <nghttp2v2/nghttp2.h>

void rand_bytes(uint8_t *dest, size_t destlen);

int begin_headers(nghttp2_conn *conn, int64_t stream_id, void *conn_user_data,
                  void *stream_user_data);

int recv_header(nghttp2_conn *conn, int64_t stream_id, int32_t token,
                nghttp2_rcbuf *name, nghttp2_rcbuf *value, uint8_t flags,
                void *conn_user_data, void *stream_user_data);

int end_headers(nghttp2_conn *conn, int64_t stream_id, int fin,
                void *conn_user_data, void *stream_user_data);

int recv_data(nghttp2_conn *conn, int64_t stream_id, const uint8_t *data,
              size_t datalen, void *conn_user_data, void *stream_user_data);

int remote_end_stream(nghttp2_conn *conn, int64_t stream_id,
                      void *conn_user_data, void *stream_user_data);

int stream_close(nghttp2_conn *conn, uint32_t flags, int64_t stream_id,
                 uint32_t error_code, void *conn_user_data,
                 void *stream_user_data);

int write_stream_data_offset(nghttp2_conn *conn, int64_t stream_id,
                             uint64_t offset, size_t len, void *conn_user_data,
                             void *stream_user_data);

nghttp2_ssize read_data(nghttp2_conn *conn, int64_t stream_id, nghttp2_vec *vec,
                        size_t veccnt, uint32_t *pflags, void *conn_user_data,
                        void *stream_user_data);

void log_write(void *user_data, char *msg, size_t len);

#endif /* CNGHTTP2_H */
