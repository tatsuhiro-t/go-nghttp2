go-nghttp2
==========

The experimental project to use nghttp2 C library from Go using cgo.
Currently, server implementation is available.

How to run demo server
----------------------

First, build and install `nghttp2 <https://nghttp2.org>`_.  Do git
clone https://github.com/tatsuhiro-t/go-nghttp2.git and enter demo
directory.  Then run ``go run demo.go``.  It fires up HTTP/2 capable
https server listening at port 3000.  It also supports HTTP/1
connection.

nghttp2.Server only supports https HTTP/2 connection right now.

Example
-------

.. code-block:: go

    package main

    import (
	    "github.com/tatsuhiro-t/go-nghttp2"
	    "io"
	    "log"
	    "net/http"
    )

    func main() {
	    var srv http.Server
	    srv.Addr = "localhost:3000"

	    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		    io.WriteString(w, "hello world")
	    })
	    // Set up srv so that if HTTP/2 is negotiated in TLS NPN,
	    // connection is intercepted by nghttp2.Server.
	    nghttp2.ConfigureServer(&srv, &nghttp2.Server{})

	    log.Printf("Listening on " + srv.Addr)
	    log.Fatal(srv.ListenAndServeTLS("server.crt", "server.key"))
    }
