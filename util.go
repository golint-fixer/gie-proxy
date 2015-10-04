package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

func shouldUpgradeWebsocket(r *http.Request) bool {
	conn_hdr := ""
	conn_hdrs := r.Header["Connection"]
	log.Printf("Connection headers: %v", conn_hdrs)
	if len(conn_hdrs) > 0 {
		conn_hdr = conn_hdrs[0]
	}
	upgrade_websocket := false
	if strings.ToLower(conn_hdr) == "upgrade" {
		log.Printf("got Connection: Upgrade")
		upgrade_hdrs := r.Header["Upgrade"]
		log.Printf("Upgrade headers: %v", upgrade_hdrs)
		if len(upgrade_hdrs) > 0 {
			upgrade_websocket = (strings.ToLower(upgrade_hdrs[0]) == "websocket")
		}
	}
	return upgrade_websocket
}

func plumbWebsocket(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}
	conn, bufrw, err := hj.Hijack()
	defer conn.Close()
	conn2, err := net.Dial("tcp", r.URL.Host)
	if err != nil {
		http.Error(w, "couldn't connect to backend server", http.StatusServiceUnavailable)
		return
	}
	defer conn2.Close()
	err = r.Write(conn2)
	if err != nil {
		log.Printf("writing WebSocket request to backend server failed: %v", err)
		return
	}
	CopyBidir(conn, bufrw, conn2, bufio.NewReadWriter(bufio.NewReader(conn2), bufio.NewWriter(conn2)))
}

func plumbHttp(h *RequestHandler2, w http.ResponseWriter, r *http.Request) {
	resp, err := h.Transport.RoundTrip(r)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "Error: %v", err)
		return
	}
	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	resp.Body.Close()
}

func Copy(dest *bufio.ReadWriter, src *bufio.ReadWriter) {
	buf := make([]byte, 40*1024)
	for {
		n, err := src.Read(buf)
		if err != nil && err != io.EOF {
			log.Printf("Read failed: %v", err)
			return
		}
		if n == 0 {
			return
		}
		dest.Write(buf[0:n])
		dest.Flush()
	}
}

func CopyBidir(conn1 io.ReadWriteCloser, rw1 *bufio.ReadWriter, conn2 io.ReadWriteCloser, rw2 *bufio.ReadWriter) {
	finished := make(chan bool)
	go func() {
		Copy(rw2, rw1)
		conn2.Close()
		finished <- true
	}()
	go func() {
		Copy(rw1, rw2)
		conn1.Close()
		finished <- true
	}()
	<-finished
	<-finished
}