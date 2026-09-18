// Guest agent: runs inside the microVM, listens on vsock port 5000, executes commands.
// Build: GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/agent ./guest
package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/avirajkhare00/sandboxd/proto"
	"golang.org/x/sys/unix"
)

const port = 5000

func main() {
	os.Setenv("PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin") // init starts with an empty env
	os.Setenv("HOME", "/root")
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
	log.Printf("agent listening on vsock:%d", port)
	// Go's net package doesn't know AF_VSOCK, so accept by hand and wrap the fd.
	for {
		nfd, _, err := unix.Accept(fd)
		if err != nil {
			log.Print(err)
			continue
		}
		go serve(os.NewFile(uintptr(nfd), "vsock-conn"))
	}
}

func serve(c io.ReadWriteCloser) {
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
	if err != nil && resp.ExitCode == -1 { // failed to start (not found, bad dir, killed)
		resp.Error = err.Error()
	}
	if ctx.Err() != nil {
		resp.Error = "timeout"
	}
	return resp
}
