package main

import (
	"context"
	"testing"
	"time"
)

func TestPoolRefills(t *testing.T) {
	n := 0
	p := NewPool(2, func() (*VM, error) { n++; return &VM{ID: "x"}, nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for i := 0; i < 5; i++ {
		if _, err := p.Get(ctx); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if n != 7 { // 2 initial + 5 refills
		t.Fatalf("made %d vms, want 7", n)
	}
}
