package pgpubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// PubSub wraps PostgreSQL LISTEN/NOTIFY for real-time event dispatch.
type PubSub struct {
	dsn      string
	listener *pq.Listener

	mu       sync.RWMutex
	handlers map[string][]func(payload string)

	cancel context.CancelFunc
	done   chan struct{}
	started bool
}

// New creates a new PubSub instance bound to the given PostgreSQL DSN.
// Call Start() to begin listening and Subscribe() to register handlers.
func New(dsn string) *PubSub {
	return &PubSub{
		dsn:      dsn,
		handlers: make(map[string][]func(payload string)),
		done:     make(chan struct{}),
	}
}

// Subscribe registers a handler for the given channel. It is safe to call
// before or after Start. If called after Start, the channel is dynamically
// added to the underlying pq.Listener.
func (ps *PubSub) Subscribe(channel string, handler func(payload string)) {
	ps.mu.Lock()
	isNew := len(ps.handlers[channel]) == 0
	ps.handlers[channel] = append(ps.handlers[channel], handler)
	started := ps.started
	ps.mu.Unlock()

	// If we are already listening and this is a brand-new channel, tell the
	// underlying pq.Listener about it.
	if started && isNew {
		if err := ps.listener.Listen(channel); err != nil {
			log.Printf("[pgpubsub] failed to LISTEN on channel %q: %v", channel, err)
		}
	}
}

// Start connects to PostgreSQL via lib/pq Listener, subscribes to all
// channels that have been registered so far, and launches the dispatch loop.
func (ps *PubSub) Start() error {
	reportProblem := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("[pgpubsub] listener event %d: %v", ev, err)
		}
	}

	ps.listener = pq.NewListener(ps.dsn, 10*time.Second, time.Minute, reportProblem)

	// Subscribe to every channel that has handlers registered before Start.
	ps.mu.RLock()
	channels := make([]string, 0, len(ps.handlers))
	for ch := range ps.handlers {
		channels = append(channels, ch)
	}
	ps.mu.RUnlock()

	for _, ch := range channels {
		if err := ps.listener.Listen(ch); err != nil {
			_ = ps.listener.Close()
			return fmt.Errorf("pgpubsub: LISTEN %q: %w", ch, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel

	ps.mu.Lock()
	ps.started = true
	ps.mu.Unlock()

	go ps.dispatchLoop(ctx)

	return nil
}

// dispatchLoop reads notifications from the pq.Listener channel and fans them
// out to registered handlers. It also issues a keep-alive Ping every 90
// seconds so the connection does not go stale.
func (ps *PubSub) dispatchLoop(ctx context.Context) {
	defer close(ps.done)

	keepAlive := time.NewTicker(90 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case n := <-ps.listener.Notify:
			// nil notifications occur during reconnection; skip them.
			if n == nil {
				continue
			}
			ps.dispatch(n.Channel, n.Extra)

		case <-keepAlive.C:
			if err := ps.listener.Ping(); err != nil {
				log.Printf("[pgpubsub] keep-alive ping failed: %v", err)
			}
		}
	}
}

// dispatch fans out a notification to every handler registered for the channel.
// Each handler is invoked in its own goroutine so that a slow handler cannot
// block the dispatch loop.
func (ps *PubSub) dispatch(channel, payload string) {
	ps.mu.RLock()
	handlers := ps.handlers[channel]
	ps.mu.RUnlock()

	for _, h := range handlers {
		go h(payload)
	}
}

// Notify sends a PostgreSQL NOTIFY on the given channel with the payload
// JSON-marshalled from the provided value. It uses GORM's Exec to run the
// SQL statement.
func Notify(db *gorm.DB, channel string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pgpubsub: marshal payload: %w", err)
	}
	return NotifyRaw(db, channel, string(data))
}

// NotifyRaw sends a PostgreSQL NOTIFY on the given channel with a raw string
// payload via GORM's Exec.
func NotifyRaw(db *gorm.DB, channel string, payload string) error {
	tx := db.Exec("SELECT pg_notify(?, ?)", channel, payload)
	if tx.Error != nil {
		return fmt.Errorf("pgpubsub: pg_notify: %w", tx.Error)
	}
	return nil
}

// Stop gracefully shuts down the dispatch loop and closes the underlying
// pq.Listener connection.
func (ps *PubSub) Stop() {
	if ps.cancel != nil {
		ps.cancel()
	}
	// Wait for the dispatch loop to exit.
	<-ps.done
	if ps.listener != nil {
		_ = ps.listener.Close()
	}
}
