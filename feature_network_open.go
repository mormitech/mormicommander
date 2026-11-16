// Opening a server or a share in the Network panel runs on its own, apart from the panel's listing. A panel's listing
// is cancelled as soon as the user goes somewhere else (feature_fs.go), but connecting to a server or mounting a share
// can take seconds, and the user asked for it: so the work goes on in the background until it succeeds or fails, a
// listing of the same folder that comes later waits for the same work instead of starting it again, and the page is
// told which folders are being opened ("network-opening"), so it can show a spinner next to them in any panel, also
// after the user went away and came back. A server's list of shares is kept for a minute: going back and forth between
// a server and its shares doesn't ask the server every time, and a share added on the server shows within a minute.
// Mounted shares need no cache: a share that is mounted is found at once the next time.
package main

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/netbrowse"
)

const (
	netOpenTimeout   = 3 * time.Minute  // the longest an open may take (macOS's sign-in window may be up meanwhile)
	netShareCacheTTL = 60 * time.Second // how long a server's list of shares is shown without asking it again
)

// netJob is a server or share being opened: done is closed when val and err are set.
type netJob struct {
	done chan struct{}
	val  any
	err  error
}

// netCachedShares is a server's list of shares and when it was read.
type netCachedShares struct {
	list []netbrowse.Share
	at   time.Time
}

// netOpens holds the opens in progress (by the folder's network path) and the cached share lists.
type netOpens struct {
	mu     sync.Mutex
	jobs   map[string]*netJob
	shares map[string]netCachedShares
}

// NetOpening is the "network-opening" event: the folders being opened now, and the one that just ended (with its
// error, "" when it opened).
type NetOpening struct {
	Opening []string `json:"opening"`
	Ended   string   `json:"ended,omitempty"`
	OK      bool     `json:"ok"`
	Error   string   `json:"error,omitempty"`
}

func (n *Network) opens() *netOpens {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.op == nil {
		n.op = &netOpens{jobs: map[string]*netJob{}, shares: map[string]netCachedShares{}}
	}
	return n.op
}

// openingNow lists the folders being opened, sorted.
func (n *Network) openingNow() []string {
	o := n.opens()
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, 0, len(o.jobs))
	for k := range o.jobs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// detached runs fn for the folder key in the background (once: a second caller for the same key waits for the same
// run) and waits for it as long as ctx lasts. When ctx ends first the caller gets its error, and fn goes on.
func (n *Network) detached(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	o := n.opens()
	o.mu.Lock()
	j := o.jobs[key]
	if j == nil {
		j = &netJob{done: make(chan struct{})}
		o.jobs[key] = j
		o.mu.Unlock()
		n.emitOpening(NetOpening{})
		go func() {
			jctx, cancel := context.WithTimeout(context.Background(), netOpenTimeout)
			defer cancel()
			j.val, j.err = fn(jctx)
			o.mu.Lock()
			delete(o.jobs, key)
			o.mu.Unlock()
			close(j.done)
			ev := NetOpening{Ended: key, OK: j.err == nil}
			if j.err != nil {
				ev.Error = j.err.Error()
			}
			n.emitOpening(ev)
		}()
	} else {
		o.mu.Unlock()
	}
	select {
	case <-j.done:
		return j.val, j.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (n *Network) emitOpening(ev NetOpening) {
	ev.Opening = n.openingNow()
	if n.app != nil {
		n.app.emit("network-opening", ev)
	}
}

// sharesOf is a server's list of shares for the Network panel: from the cache when it was read less than a minute ago,
// else read in the background (detached), so leaving the folder doesn't stop it.
func (n *Network) sharesOf(ctx context.Context, key string, t netbrowse.Target, c netbrowse.Creds) ([]netbrowse.Share, error) {
	ck := t.Kind + "\x00" + strings.ToLower(t.Host) + "\x00" + c.User
	o := n.opens()
	o.mu.Lock()
	if e, ok := o.shares[ck]; ok && time.Since(e.at) < netShareCacheTTL {
		o.mu.Unlock()
		return e.list, nil
	}
	o.mu.Unlock()
	v, err := n.detached(ctx, key, func(jctx context.Context) (any, error) {
		list, err := n.shares(jctx, t, c)
		if err == nil {
			o.mu.Lock()
			o.shares[ck] = netCachedShares{list, time.Now()}
			o.mu.Unlock()
		}
		return list, err
	})
	if err != nil {
		return nil, err
	}
	return v.([]netbrowse.Share), nil
}

// mountOf mounts a share for the Network panel in the background (detached); a share the app mounted is noted, so it
// is disconnected when the app ends, also when the user had gone elsewhere by the time it was mounted.
func (n *Network) mountOf(ctx context.Context, key string, t netbrowse.Target, c netbrowse.Creds) (netbrowse.Mounted, error) {
	v, err := n.detached(ctx, key, func(jctx context.Context) (any, error) {
		m, err := n.mountShare(jctx, t, c, netbrowse.MountOptions{})
		if err == nil && m.Made {
			n.mu.Lock()
			n.temps = append(n.temps, m.Path)
			n.mu.Unlock()
			if n.app != nil {
				n.app.emit("system-volumes")
			}
		}
		return m, err
	})
	if err != nil {
		return netbrowse.Mounted{}, err
	}
	return v.(netbrowse.Mounted), nil
}
