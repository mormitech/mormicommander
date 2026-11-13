// This file keeps idle FTP connections alive. Servers and NAT routers drop
// control connections that stay silent for a few minutes, which would make
// the next click in an FTP panel fail; Total Commander therefore sends NOOP
// after a configurable idle time. The keepalive goroutine never waits for a
// busy connection: if a transfer or listing holds the control lock it simply
// tries again later, because the busy connection is obviously not idle.
package ftp

import "time"

// StartKeepalive sends NOOP whenever the connection has been idle for
// interval. Calling it again replaces the previous interval.
func (c *Client) StartKeepalive(interval time.Duration) {
	c.StopKeepalive()
	if interval <= 0 {
		return
	}
	stop := make(chan struct{})
	c.kaMu.Lock()
	c.kaStop = stop
	c.kaMu.Unlock()
	tick := interval / 4
	if tick < 10*time.Millisecond {
		tick = 10 * time.Millisecond
	}
	if tick > 5*time.Second {
		tick = 5 * time.Second
	}
	go func() {
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
			}
			if c.IdleFor() < interval || !c.mu.TryLock() {
				continue
			}
			if c.broken == nil {
				c.raw("NOOP")
				c.conn.SetDeadline(time.Time{})
			}
			c.touch()
			c.mu.Unlock()
		}
	}()
}

// StopKeepalive ends the keepalive goroutine, if any.
func (c *Client) StopKeepalive() {
	c.kaMu.Lock()
	defer c.kaMu.Unlock()
	if c.kaStop != nil {
		close(c.kaStop)
		c.kaStop = nil
	}
}
