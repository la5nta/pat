// Copyright 2016 Martin Hebnes Pedersen (LA5NTA). All rights reserved.
// Use of this source code is governed by the MIT-license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"net"
	"os"
	"time"
)

type EventLogger struct {
	file *os.File
	enc  *json.Encoder
}

func NewEventLogger(path string) (*EventLogger, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	return &EventLogger{file, json.NewEncoder(file)}, err
}

func (l *EventLogger) Close() error { return l.file.Close() }

func (l *EventLogger) Log(what string, event map[string]interface{}) {
	event["log_time"] = time.Now()
	event["what"] = what

	if err := l.enc.Encode(event); err != nil {
		panic(err)
	}
}

func (l *EventLogger) LogConn(op string, freq Frequency, conn net.Conn, err error) {
	e := map[string]interface{}{"success": err == nil}

	if err != nil {
		e["error"] = err.Error()
	} else {
		if remote := conn.RemoteAddr(); remote != nil {
			e["remote_addr"] = remote.String()
			e["network"] = conn.RemoteAddr().Network()
		}
		if local := conn.LocalAddr(); local != nil {
			e["local_addr"] = local.String()
		}
	}

	if freq > 0 {
		e["freq"] = freq
	}

	e["operation"] = op

	l.Log("connect", e)
}
