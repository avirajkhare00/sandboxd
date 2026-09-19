// Package proto is the newline-delimited JSON wire format between host and guest agent.
package proto

import (
	"bufio"
	"encoding/json"
	"io"
)

type Req struct {
	Cmd     []string `json:"cmd"`
	Stdin   string   `json:"stdin,omitempty"`
	Timeout int      `json:"timeout_ms,omitempty"`
	NowNs   int64    `json:"now_ns,omitempty"` // host wall clock; restored guests wake with a stale clock
}

type Resp struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

func Write(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

func Read(r *bufio.Reader, v any) error {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, v)
}
