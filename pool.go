package main

import (
	"context"
	"log"
	"time"
)

// Pool keeps N ready VMs. Get hands one out and triggers a background refill.
type Pool struct {
	ready chan *VM
	make  func() (*VM, error)
}

func NewPool(n int, make func() (*VM, error)) *Pool {
	p := &Pool{ready: make0(n), make: make}
	for i := 0; i < n; i++ {
		go p.fill()
	}
	return p
}

func make0(n int) chan *VM { return make(chan *VM, n) }

func (p *Pool) fill() {
	for {
		vm, err := p.make()
		if err != nil {
			log.Printf("pool fill: %v", err)
			time.Sleep(time.Second)
			continue
		}
		p.ready <- vm
		return
	}
}

func (p *Pool) Get(ctx context.Context) (*VM, error) {
	select {
	case vm := <-p.ready:
		go p.fill()
		return vm, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
