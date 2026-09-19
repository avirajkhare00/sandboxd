// loadtest: create N sandboxes concurrently, run M execs in each, report latency percentiles.
// go run ./cmd/loadtest -n 10 -m 5 -cmd 'python3 -c "print(1)"'
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

var (
	addr = flag.String("addr", "http://127.0.0.1:8080", "sandboxd address")
	n    = flag.Int("n", 10, "sandboxes to create concurrently")
	m    = flag.Int("m", 5, "execs per sandbox, sequential")
	cmd  = flag.String("cmd", `python3 -c "print(1)"`, "shell command to exec")
	keep = flag.Bool("keep", false, "leave sandboxes running (for memory measurement)")
)

type sample struct {
	create []time.Duration
	exec   []time.Duration
	errs   int
	mu     sync.Mutex
}

func main() {
	flag.Parse()
	var s sample
	var wg sync.WaitGroup
	t0 := time.Now()
	for i := 0; i < *n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tc := time.Now()
			id, err := create()
			if err != nil {
				s.mu.Lock()
				s.errs++
				s.mu.Unlock()
				fmt.Fprintln(os.Stderr, "create:", err)
				return
			}
			s.mu.Lock()
			s.create = append(s.create, time.Since(tc))
			s.mu.Unlock()
			for j := 0; j < *m; j++ {
				te := time.Now()
				if err := exec(id, *cmd); err != nil {
					s.mu.Lock()
					s.errs++
					s.mu.Unlock()
					fmt.Fprintln(os.Stderr, "exec:", err)
					continue
				}
				s.mu.Lock()
				s.exec = append(s.exec, time.Since(te))
				s.mu.Unlock()
			}
			if !*keep {
				req, _ := http.NewRequest("DELETE", *addr+"/sandboxes/"+id, nil)
				http.DefaultClient.Do(req)
			}
		}()
	}
	wg.Wait()
	fmt.Printf("n=%d m=%d wall=%s errs=%d\n", *n, *m, time.Since(t0).Round(time.Millisecond), s.errs)
	fmt.Printf("create: %s\n", pct(s.create))
	fmt.Printf("exec:   %s\n", pct(s.exec))
}

func create() (string, error) {
	resp, err := http.Post(*addr+"/sandboxes", "application/json", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var v struct {
		ID string `json:"id"`
	}
	return v.ID, json.NewDecoder(resp.Body).Decode(&v)
}

func exec(id, sh string) error {
	body, _ := json.Marshal(map[string]any{"cmd": []string{"sh", "-c", sh}})
	resp, err := http.Post(*addr+"/sandboxes/"+id+"/exec", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var v struct {
		ExitCode int    `json:"exit_code"`
		Error    string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return err
	}
	if v.ExitCode != 0 {
		return fmt.Errorf("exit %d %s", v.ExitCode, v.Error)
	}
	return nil
}

func pct(d []time.Duration) string {
	if len(d) == 0 {
		return "no samples"
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	p := func(q float64) time.Duration { return d[int(float64(len(d)-1)*q)].Round(time.Millisecond) }
	return fmt.Sprintf("n=%d p50=%s p90=%s p99=%s max=%s", len(d), p(.5), p(.9), p(.99), p(1))
}
