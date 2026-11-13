// This file provides a small connection pool for one server. FTP can only
// do one thing at a time per connection, so Total Commander's background
// transfer queue opens extra connections to copy several files in parallel
// while the panel keeps browsing on its own connection. The pool hands out
// at most Max clients, reuses idle ones, throws away connections that broke,
// and can close connections that sat unused for too long (the "disconnect
// after idle time" setting).
package ftp

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Pool hands out logged-in clients for one Config.
type Pool struct {
	cfg  Config
	sem  chan struct{}
	mu   sync.Mutex
	idle []*Client
	dial func(ctx context.Context, cfg Config) (*Client, error)
	done bool
}

// NewPool creates a pool allowing at most max simultaneous connections
// (max < 1 is treated as 1).
func NewPool(cfg Config, max int) *Pool {
	if max < 1 {
		max = 1
	}
	return &Pool{cfg: cfg, sem: make(chan struct{}, max), dial: Dial}
}

// Get returns an idle client or dials a new one, waiting while Max
// clients are in use.
func (p *Pool) Get(ctx context.Context) (*Client, error) {
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		<-p.sem
		return nil, errors.New("ftp: pool closed")
	}
	for len(p.idle) > 0 {
		c := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		if c.Broken() == nil {
			p.mu.Unlock()
			return c, nil
		}
		c.Close()
	}
	p.mu.Unlock()
	c, err := p.dial(ctx, p.cfg)
	if err != nil {
		<-p.sem
		return nil, err
	}
	return c, nil
}

// Put returns a client obtained from Get. Broken clients are closed.
func (p *Pool) Put(c *Client) {
	if c == nil {
		return
	}
	p.mu.Lock()
	if p.done || c.Broken() != nil {
		p.mu.Unlock()
		c.Close()
	} else {
		p.idle = append(p.idle, c)
		p.mu.Unlock()
	}
	<-p.sem
}

// Idle returns the number of idle connections.
func (p *Pool) Idle() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.idle)
}

// CloseIdle disconnects idle clients unused for at least olderThan and
// returns how many were closed.
func (p *Pool) CloseIdle(olderThan time.Duration) int {
	p.mu.Lock()
	var keep, drop []*Client
	for _, c := range p.idle {
		if c.IdleFor() >= olderThan {
			drop = append(drop, c)
		} else {
			keep = append(keep, c)
		}
	}
	p.idle = keep
	p.mu.Unlock()
	for _, c := range drop {
		c.Close()
	}
	return len(drop)
}

// Close disconnects all idle clients; clients still in use are closed when
// they are put back.
func (p *Pool) Close() {
	p.mu.Lock()
	p.done = true
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()
	for _, c := range idle {
		c.Close()
	}
}
