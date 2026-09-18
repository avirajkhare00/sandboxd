// sandboxd: Firecracker microVM sandbox service for agent harnesses.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/avirajkhare00/sandboxd/proto"
)

var cfg = struct {
	Listen                                  string
	Firecracker, Jailer, Kernel, BaseRootfs string
	SnapDir                                 string
	StateDir                                string
	PoolSize                                int
	MaxLife                                 time.Duration
	VCPUs                                   int64
	MemMiB                                  int64
	Egress                                  string
}{}

func main() {
	flag.StringVar(&cfg.Listen, "listen", "127.0.0.1:8080", "http listen addr")
	flag.StringVar(&cfg.Firecracker, "firecracker", "/usr/local/bin/firecracker", "firecracker binary")
	flag.StringVar(&cfg.Jailer, "jailer", "/usr/local/bin/jailer", "jailer binary")
	flag.StringVar(&cfg.Kernel, "kernel", "build/vmlinux", "guest kernel")
	flag.StringVar(&cfg.BaseRootfs, "rootfs", "build/rootfs.ext4", "base rootfs (read-only)")
	flag.StringVar(&cfg.SnapDir, "snapdir", "build/snapshot", "dir with vm.snap + vm.mem (empty = cold boot)")
	flag.StringVar(&cfg.StateDir, "state", "/srv/sandboxd", "jailer chroot base + overlays")
	flag.IntVar(&cfg.PoolSize, "pool", 4, "warm pool size")
	flag.DurationVar(&cfg.MaxLife, "maxlife", 15*time.Minute, "max sandbox lifetime")
	flag.Int64Var(&cfg.VCPUs, "vcpus", 1, "vcpus per vm")
	flag.Int64Var(&cfg.MemMiB, "mem", 512, "memory MiB per vm")
	flag.StringVar(&cfg.Egress, "egress", "", "comma-separated CIDRs allowed for egress (empty = deny all)")
	flag.Parse()

	if err := setupHostNet(strings.Split(cfg.Egress, ",")); err != nil {
		log.Fatal(err)
	}
	pool := NewPool(cfg.PoolSize, newVM)
	s := &server{pool: pool, live: map[string]*VM{}}
	log.Printf("sandboxd listening on %s", cfg.Listen)
	log.Fatal(http.ListenAndServe(cfg.Listen, s.routes()))
}

type server struct {
	pool *Pool
	mu   sync.Mutex
	live map[string]*VM
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sandboxes", s.create)
	mux.HandleFunc("POST /sandboxes/{id}/exec", s.exec)
	mux.HandleFunc("PUT /sandboxes/{id}/files/{path...}", s.putFile)
	mux.HandleFunc("DELETE /sandboxes/{id}", s.delete)
	return mux
}

func (s *server) get(w http.ResponseWriter, r *http.Request) (*VM, bool) {
	s.mu.Lock()
	vm, ok := s.live[r.PathValue("id")]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "not found", 404)
	}
	return vm, ok
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	vm, err := s.pool.Get(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	s.mu.Lock()
	s.live[vm.ID] = vm
	s.mu.Unlock()
	time.AfterFunc(cfg.MaxLife, func() { s.destroy(vm.ID) })
	writeJSON(w, map[string]string{"id": vm.ID})
}

func (s *server) exec(w http.ResponseWriter, r *http.Request) {
	vm, ok := s.get(w, r)
	if !ok {
		return
	}
	var req proto.Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	resp, err := vm.Exec(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	writeJSON(w, resp)
}

func (s *server) putFile(w http.ResponseWriter, r *http.Request) {
	vm, ok := s.get(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// ponytail: file upload rides on exec + stdin; add a real file channel if 8MB isn't enough.
	path := "/work/" + r.PathValue("path")
	resp, err := vm.Exec(r.Context(), proto.Req{Cmd: []string{"sh", "-c", "mkdir -p \"$(dirname \"$0\")\" && cat > \"$0\"", path}, Stdin: string(body)})
	if err != nil || resp.ExitCode != 0 {
		http.Error(w, "write failed", 502)
		return
	}
	w.WriteHeader(204)
}

func (s *server) delete(w http.ResponseWriter, r *http.Request) {
	s.destroy(r.PathValue("id"))
	w.WriteHeader(204)
}

func (s *server) destroy(id string) {
	s.mu.Lock()
	vm, ok := s.live[id]
	delete(s.live, id)
	s.mu.Unlock()
	if ok {
		if err := vm.Destroy(); err != nil {
			log.Printf("destroy %s: %v", id, err)
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
