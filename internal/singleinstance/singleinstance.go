// Package singleinstance keeps a second launch of Koalmine from opening a
// second tray icon and a second hidden window. It uses a fixed localhost TCP
// port as both the lock (whoever binds it first is the primary instance) and
// the IPC channel a later launch uses to ask the primary to show its window.
//
// A named Windows mutex would be the more idiomatic lock on the only
// platform this app is actually tested on, but it can't also carry the
// "please show your window" message — that would still need a second IPC
// mechanism (a named pipe, most likely). A single localhost socket does
// both jobs with one small, cross-platform primitive instead of two
// platform-specific ones.
package singleinstance

import (
	"bufio"
	"net"
	"time"
)

// addr is fixed and specific enough to avoid colliding with other local
// services. Binding only 127.0.0.1 keeps it unreachable from the network.
const addr = "127.0.0.1:58743"

const showCommand = "show"

// Acquire tries to become the primary Koalmine instance.
//
// If it succeeds, it returns true and onShow will be invoked (from its own
// goroutine, once per request) every time a later launch asks to be shown.
//
// If another instance already holds the lock, Acquire forwards a "show
// window" request to it and returns false — the caller should exit
// immediately rather than starting a second tray icon/window.
func Acquire(onShow func()) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		notifyExisting()
		return false
	}
	go serve(ln, onShow)
	return true
}

func notifyExisting() {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		// Nothing we can do — the port might be held by something other
		// than Koalmine. Let the caller proceed as a normal (if redundant)
		// launch rather than blocking startup on this.
		return
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	conn.Write([]byte(showCommand + "\n"))
}

func serve(ln net.Listener, onShow func()) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handle(conn, onShow)
	}
}

func handle(conn net.Conn, onShow func()) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		if scanner.Text() == showCommand && onShow != nil {
			onShow()
		}
	}
}
