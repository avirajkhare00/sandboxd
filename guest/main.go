// Guest agent: runs inside the microVM, listens on vsock port 5000, executes commands.
// Build: GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/agent ./guest
package main

import (
	"bufio"
	"bytes"
	"context"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/avirajkhare00/sandboxd/proto"
	"golang.org/x/sys/unix"
)

const port = 5000

func main() {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		log.Fatal(err)
	}
	if err := unix.Bind(fd, &unix.SockaddrVM{CID: unix.VMADDR_CID_ANY, Port: port}); err != nil {
		log.Fatal(err)
	}
	if err := unix.Listen(fd, 16); err != nil {
		log.Fatal(err)
	}
	f := os.NewFile(uintptr(fd), "vsock")
	ln, err := net.FileListener(f)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("agent listening on vsock:%d", port)
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Print(err)
			continue
		}
		go serve(c)
	}
}

func serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	for {
		var req proto.Req
		if err := proto.Read(r, &req); err != nil {
			return
		}
		proto.Write(c, run(req))
	}
}

func run(req proto.Req) proto.Resp {
	if len(req.Cmd) == 0 {
		return proto.Resp{Error: "empty cmd", ExitCode: -1}
	}
	timeout := time.Duration(req.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, req.Cmd[0], req.Cmd[1:]...)
	cmd.Dir = "/work"
	cmd.Stdin = bytes.NewBufferString(req.Stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	resp := proto.Resp{Stdout: out.String(), Stderr: errb.String(), ExitCode: cmd.ProcessState.ExitCode()}
	if err != nil && resp.ExitCode == 0 {
		resp.Error = err.Error()
		resp.ExitCode = -1
	}
	if ctx.Err() != nil {
		resp.Error = "timeout"
	}
	return resp
}
