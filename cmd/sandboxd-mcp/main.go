// sandboxd-mcp: MCP server (stdio) exposing sandboxd to Claude Code and other MCP clients.
//
//	{"mcpServers":{"sandbox":{"command":"sandboxd-mcp","env":{"SANDBOXD_ADDR":"http://127.0.0.1:8080"}}}}
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var addr = env("SANDBOXD_ADDR", "http://127.0.0.1:8080")

type ExecOut struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

type RunIn struct {
	Cmd       string `json:"cmd" jsonschema:"shell command to run with sh -c inside the sandbox; cwd is /work"`
	Stdin     string `json:"stdin,omitempty" jsonschema:"optional stdin"`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"kill after this many ms (default 60000)"`
	Sandbox   string `json:"sandbox,omitempty" jsonschema:"sandbox id from sandbox_create; omit to use the session sandbox"`
}

type PutIn struct {
	Path    string `json:"path" jsonschema:"path relative to /work"`
	Content string `json:"content" jsonschema:"file contents"`
	Sandbox string `json:"sandbox,omitempty" jsonschema:"sandbox id; omit to use the session sandbox"`
}

type IDIn struct {
	Sandbox string `json:"sandbox" jsonschema:"sandbox id"`
}
type IDOut struct {
	Sandbox string `json:"sandbox"`
}

// session sandbox: created on first use so the model needn't manage ids.
var (
	mu      sync.Mutex
	session string
)

func sessionID(ctx context.Context) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if session == "" {
		id, err := create(ctx)
		if err != nil {
			return "", err
		}
		session = id
	}
	return session, nil
}

func pick(ctx context.Context, id string) (string, error) {
	if id != "" {
		return id, nil
	}
	return sessionID(ctx)
}

func main() {
	s := mcp.NewServer(&mcp.Implementation{Name: "sandboxd", Version: "0.1.0"}, nil)

	mcp.AddTool(s, &mcp.Tool{Name: "sandbox_run", Description: "Run a shell command inside an isolated Firecracker microVM. Files written to /work persist for the session. Use this instead of running untrusted or model-written code on the host."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in RunIn) (*mcp.CallToolResult, ExecOut, error) {
			id, err := pick(ctx, in.Sandbox)
			if err != nil {
				return nil, ExecOut{}, err
			}
			out, err := exec(ctx, id, []string{"sh", "-c", in.Cmd}, in.Stdin, in.TimeoutMs)
			return nil, out, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sandbox_put_file", Description: "Write a file into the sandbox under /work. Parent directories are created."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in PutIn) (*mcp.CallToolResult, IDOut, error) {
			id, err := pick(ctx, in.Sandbox)
			if err != nil {
				return nil, IDOut{}, err
			}
			req, _ := http.NewRequestWithContext(ctx, "PUT", addr+"/sandboxes/"+id+"/files/"+in.Path, bytes.NewBufferString(in.Content))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, IDOut{}, err
			}
			resp.Body.Close()
			if resp.StatusCode != 204 {
				return nil, IDOut{}, fmt.Errorf("put_file: status %d", resp.StatusCode)
			}
			return nil, IDOut{Sandbox: id}, nil
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sandbox_create", Description: "Create an additional isolated sandbox and return its id. Only needed when you want more than the default session sandbox."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, IDOut, error) {
			id, err := create(ctx)
			return nil, IDOut{Sandbox: id}, err
		})

	mcp.AddTool(s, &mcp.Tool{Name: "sandbox_destroy", Description: "Destroy a sandbox and everything in it."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in IDIn) (*mcp.CallToolResult, IDOut, error) {
			req, _ := http.NewRequestWithContext(ctx, "DELETE", addr+"/sandboxes/"+in.Sandbox, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, IDOut{}, err
			}
			resp.Body.Close()
			mu.Lock()
			if session == in.Sandbox {
				session = ""
			}
			mu.Unlock()
			return nil, IDOut{Sandbox: in.Sandbox}, nil
		})

	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func create(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", addr+"/sandboxes", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sandboxd at %s: %w", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create: %d %s", resp.StatusCode, b)
	}
	var v struct {
		ID string `json:"id"`
	}
	return v.ID, json.NewDecoder(resp.Body).Decode(&v)
}

func exec(ctx context.Context, id string, cmd []string, stdin string, timeoutMs int) (ExecOut, error) {
	body, _ := json.Marshal(map[string]any{"cmd": cmd, "stdin": stdin, "timeout_ms": timeoutMs})
	req, _ := http.NewRequestWithContext(ctx, "POST", addr+"/sandboxes/"+id+"/exec", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ExecOut{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return ExecOut{}, fmt.Errorf("exec: %d %s", resp.StatusCode, b)
	}
	var out ExecOut
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
