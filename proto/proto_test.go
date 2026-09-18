package proto

import (
	"bufio"
	"bytes"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := Req{Cmd: []string{"echo", "hi\nthere"}, Stdin: "x"}
	if err := Write(&buf, in); err != nil {
		t.Fatal(err)
	}
	var out Req
	if err := Read(bufio.NewReader(&buf), &out); err != nil {
		t.Fatal(err)
	}
	if out.Cmd[1] != "hi\nthere" || out.Stdin != "x" {
		t.Fatalf("got %+v", out)
	}
}
