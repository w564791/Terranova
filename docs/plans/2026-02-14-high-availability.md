# High Availability Multi-Replica Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable multi-replica K8s deployment with leader election for background tasks, PG advisory locks for distributed mutual exclusion, and PG NOTIFY/LISTEN for cross-replica communication.

**Architecture:** K8s Lease leader election controls which replica runs background schedulers. PG advisory locks replace in-memory sync.Map for workspace task serialization. PG NOTIFY/LISTEN bridges WebSocket messages and agent task dispatch across replicas.

**Tech Stack:** Go, client-go leaderelection, PostgreSQL advisory locks, PostgreSQL NOTIFY/LISTEN via lib/pq, GORM, K8s Lease API

**Design Doc:** `docs/high-availability-design.md`

---

## Task 1: Create `pkg/leaderelection` module

**Files:**
- Create: `backend/pkg/leaderelection/leader.go`
- Create: `backend/pkg/leaderelection/leader_test.go`

**Step 1: Write the leader election wrapper**

```go
// backend/pkg/leaderelection/leader.go
package leaderelection

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type LeaderCallbacks struct {
	OnStartedLeading func(ctx context.Context)
	OnStoppedLeading func()
	OnNewLeader      func(identity string)
}

type Config struct {
	LeaseName      string
	LeaseNamespace string
	Identity       string // Pod name
	LeaseDuration  time.Duration
	RenewDeadline  time.Duration
	RetryPeriod    time.Duration
}

func DefaultConfig() Config {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
	return Config{
		LeaseName:      "iac-platform-leader",
		LeaseNamespace: namespace,
		Identity:       podName,
		LeaseDuration:  15 * time.Second,
		RenewDeadline:  10 * time.Second,
		RetryPeriod:    2 * time.Second,
	}
}

func newK8sClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig for local development
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			return nil, fmt.Errorf("not in cluster and KUBECONFIG not set: %w", err)
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
		}
	}
	return kubernetes.NewForConfig(config)
}

// Run starts the leader election loop. It blocks until ctx is cancelled.
// OnStartedLeading is called with a context that is cancelled when leadership is lost.
func Run(ctx context.Context, cfg Config, callbacks LeaderCallbacks) error {
	client, err := newK8sClient()
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LeaseName,
			Namespace: cfg.LeaseNamespace,
		},
		Client: client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: cfg.Identity,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   cfg.LeaseDuration,
		RenewDeadline:   cfg.RenewDeadline,
		RetryPeriod:     cfg.RetryPeriod,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderElectionCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				log.Printf("[LeaderElection] This pod (%s) is now the leader", cfg.Identity)
				if callbacks.OnStartedLeading != nil {
					callbacks.OnStartedLeading(ctx)
				}
			},
			OnStoppedLeading: func() {
				log.Printf("[LeaderElection] This pod (%s) lost leadership", cfg.Identity)
				if callbacks.OnStoppedLeading != nil {
					callbacks.OnStoppedLeading()
				}
			},
			OnNewLeader: func(identity string) {
				if identity == cfg.Identity {
					return
				}
				log.Printf("[LeaderElection] New leader elected: %s", identity)
				if callbacks.OnNewLeader != nil {
					callbacks.OnNewLeader(identity)
				}
			},
		},
	})
	return nil
}

// RunWithFallback tries leader election; if K8s is not available (local dev),
// runs as leader directly.
func RunWithFallback(ctx context.Context, callbacks LeaderCallbacks) {
	cfg := DefaultConfig()

	go func() {
		err := Run(ctx, cfg, callbacks)
		if err != nil {
			log.Printf("[LeaderElection] K8s leader election unavailable (%v), running as standalone leader", err)
			// In non-K8s environments, this instance is the leader
			if callbacks.OnStartedLeading != nil {
				callbacks.OnStartedLeading(ctx)
			}
		}
	}()
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./pkg/leaderelection/`
Expected: No errors

**Step 3: Commit**

```bash
git add backend/pkg/leaderelection/leader.go
git commit -m "feat(ha): add K8s Lease leader election wrapper"
```

---

## Task 2: Create `pkg/pglock` module

**Files:**
- Create: `backend/pkg/pglock/advisory_lock.go`

**Step 1: Write the PG advisory lock wrapper**

```go
// backend/pkg/pglock/advisory_lock.go
package pglock

import (
	"fmt"

	"gorm.io/gorm"
)

// Locker provides distributed locking via PostgreSQL advisory locks.
// Advisory locks are automatically released when the database connection/session ends,
// which means if a Pod crashes, its locks are released.
type Locker struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Locker {
	return &Locker{db: db}
}

// TryLock attempts to acquire an advisory lock (non-blocking).
// Returns true if the lock was acquired, false if it's held by another session.
// The key is a unique int64 identifier (e.g., workspace ID).
func (l *Locker) TryLock(key int64) (bool, error) {
	var locked bool
	err := l.db.Raw("SELECT pg_try_advisory_lock(?)", key).Scan(&locked).Error
	if err != nil {
		return false, fmt.Errorf("pg_try_advisory_lock(%d): %w", key, err)
	}
	return locked, nil
}

// Unlock releases an advisory lock.
// Returns true if the lock was held and released, false if it was not held.
func (l *Locker) Unlock(key int64) (bool, error) {
	var released bool
	err := l.db.Raw("SELECT pg_advisory_unlock(?)", key).Scan(&released).Error
	if err != nil {
		return false, fmt.Errorf("pg_advisory_unlock(%d): %w", key, err)
	}
	return released, nil
}

// TryLockDual attempts to acquire an advisory lock with two int32 keys.
// Useful when you need composite keys (e.g., resource_type + resource_id).
func (l *Locker) TryLockDual(key1, key2 int32) (bool, error) {
	var locked bool
	err := l.db.Raw("SELECT pg_try_advisory_lock(?, ?)", key1, key2).Scan(&locked).Error
	if err != nil {
		return false, fmt.Errorf("pg_try_advisory_lock(%d, %d): %w", key1, key2, err)
	}
	return locked, nil
}

// UnlockDual releases an advisory lock with two int32 keys.
func (l *Locker) UnlockDual(key1, key2 int32) (bool, error) {
	var released bool
	err := l.db.Raw("SELECT pg_advisory_unlock(?, ?)", key1, key2).Scan(&released).Error
	if err != nil {
		return false, fmt.Errorf("pg_advisory_unlock(%d, %d): %w", key1, key2, err)
	}
	return released, nil
}
```

**Step 2: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./pkg/pglock/`
Expected: No errors

**Step 3: Commit**

```bash
git add backend/pkg/pglock/advisory_lock.go
git commit -m "feat(ha): add PG advisory lock wrapper"
```

---

## Task 3: Create `pkg/pgpubsub` module

**Files:**
- Create: `backend/pkg/pgpubsub/pubsub.go`

**Context:** Uses `github.com/lib/pq` (already in go.mod) for native PostgreSQL LISTEN/NOTIFY support. GORM's connection pool doesn't support LISTEN, so we need a dedicated raw connection via lib/pq.

**Step 1: Write the PG NOTIFY/LISTEN wrapper**

```go
// backend/pkg/pgpubsub/pubsub.go
package pgpubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lib/pq"
)

// PubSub provides cross-replica messaging via PostgreSQL NOTIFY/LISTEN.
type PubSub struct {
	dsn       string
	listener  *pq.Listener
	handlers  map[string][]func(payload string)
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// New creates a PubSub instance. dsn is the PostgreSQL connection string.
func New(dsn string) *PubSub {
	ctx, cancel := context.WithCancel(context.Background())
	return &PubSub{
		dsn:      dsn,
		handlers: make(map[string][]func(payload string)),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start connects to PostgreSQL and begins listening.
// Call this once at application startup.
func (ps *PubSub) Start() error {
	reportProblem := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("[PGPubSub] Listener error: %v", err)
		}
	}

	ps.listener = pq.NewListener(ps.dsn, 10*time.Second, time.Minute, reportProblem)

	// Subscribe to all registered channels
	ps.mu.RLock()
	for channel := range ps.handlers {
		if err := ps.listener.Listen(channel); err != nil {
			ps.mu.RUnlock()
			return fmt.Errorf("failed to listen on channel %s: %w", channel, err)
		}
	}
	ps.mu.RUnlock()

	// Start the dispatch loop
	go ps.dispatchLoop()

	return nil
}

// Subscribe registers a handler for a channel. Must be called before Start().
// If called after Start(), the new channel will be added dynamically.
func (ps *PubSub) Subscribe(channel string, handler func(payload string)) {
	ps.mu.Lock()
	ps.handlers[channel] = append(ps.handlers[channel], handler)
	ps.mu.Unlock()

	// If listener is already running, subscribe dynamically
	if ps.listener != nil {
		if err := ps.listener.Listen(channel); err != nil {
			log.Printf("[PGPubSub] Failed to listen on channel %s: %v", channel, err)
		}
	}
}

// Notify sends a message on a channel. Can be called from any goroutine.
// Uses a separate connection (via GORM db) to avoid blocking the listener.
func (ps *PubSub) Notify(db interface{ Exec(string, ...interface{}) error }, channel string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	return db.Exec("SELECT pg_notify($1, $2)", channel, string(data))
}

// NotifyRaw sends a raw string payload.
func (ps *PubSub) NotifyRaw(db interface{ Exec(string, ...interface{}) error }, channel, payload string) error {
	return db.Exec("SELECT pg_notify($1, $2)", channel, payload)
}

func (ps *PubSub) dispatchLoop() {
	for {
		select {
		case <-ps.ctx.Done():
			return
		case notification := <-ps.listener.Notify:
			if notification == nil {
				// Reconnection happened, notification is nil
				continue
			}
			ps.mu.RLock()
			handlers := ps.handlers[notification.Channel]
			ps.mu.RUnlock()

			for _, h := range handlers {
				// Run handlers in goroutines to avoid blocking the dispatch loop
				go h(notification.Extra)
			}
		case <-time.After(90 * time.Second):
			// Ping to keep the connection alive
			go func() {
				if err := ps.listener.Ping(); err != nil {
					log.Printf("[PGPubSub] Ping failed: %v", err)
				}
			}()
		}
	}
}

// Stop closes the listener connection.
func (ps *PubSub) Stop() {
	ps.cancel()
	if ps.listener != nil {
		ps.listener.Close()
	}
}
```

**Step 2: Update the Notify method to work with GORM**

The `Notify` method uses a generic `Exec` interface. For GORM usage, callers will wrap it:

```go
// Usage example (caller side):
type gormExecer struct{ db *gorm.DB }
func (g gormExecer) Exec(sql string, args ...interface{}) error {
    return g.db.Exec(sql, args...).Error
}
ps.Notify(gormExecer{db}, "channel", payload)
```

Actually, let's simplify by just accepting `*gorm.DB` directly. Revise the Notify signature:

```go
// Revised Notify - accepts *gorm.DB directly
func (ps *PubSub) Notify(db *gorm.DB, channel string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	return db.Exec("SELECT pg_notify(?, ?)", channel, string(data)).Error
}
```

**Step 3: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./pkg/pgpubsub/`
Expected: No errors

**Step 4: Commit**

```bash
git add backend/pkg/pgpubsub/pubsub.go
git commit -m "feat(ha): add PG NOTIFY/LISTEN pubsub wrapper"
```

---

## Task 4: Add `connected_pod` field to Agent model

**Files:**
- Modify: `backend/internal/models/agent.go`

**Step 1: Add the field**

Add `ConnectedPod` field to the `Agent` struct in `backend/internal/models/agent.go`:

```go
ConnectedPod *string `gorm:"column:connected_pod;type:varchar(100)" json:"connected_pod"`
```

**Step 2: Create database migration**

Run SQL migration to add the column:

```sql
ALTER TABLE agents ADD COLUMN IF NOT EXISTS connected_pod VARCHAR(100);
```

This can be done via auto-migrate or a manual migration script.

**Step 3: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./...`
Expected: No errors

**Step 4: Commit**

```bash
git add backend/internal/models/agent.go
git commit -m "feat(ha): add connected_pod field to Agent model"
```

---

## Task 5: Add context support to schedulers that lack it

**Files:**
- Modify: `backend/services/drift_check_scheduler.go:37` — `Start(interval)` → `Start(ctx, interval)`
- Modify: `backend/services/cmdb_sync_scheduler.go:35` — `Start(checkInterval)` → `Start(ctx, checkInterval)`
- Modify: `backend/services/agent_cleanup_service.go:30` — `Start(interval)` → `Start(ctx, interval)`
- Modify: `backend/main.go` — Update callers of these 3 methods

**Context:** These 3 services currently run `go func()` with `for range ticker.C`. They need `context.Context` to support cancellation when leadership is lost. The other services (`RunTaskTimeoutChecker`, `EmbeddingWorker`, `K8sDeploymentService`, `TaskQueueManager`) already accept context.

**Step 1: Modify DriftCheckScheduler.Start**

In `backend/services/drift_check_scheduler.go`, change the `Start` method to accept context:

```go
// Before:
func (s *DriftCheckScheduler) Start(interval time.Duration) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for range ticker.C {
            // ...
        }
    }()
}

// After:
func (s *DriftCheckScheduler) Start(ctx context.Context, interval time.Duration) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                log.Println("[DriftCheckScheduler] Stopped: context cancelled")
                return
            case <-ticker.C:
                // ... existing logic unchanged
            }
        }
    }()
}
```

**Step 2: Modify CMDBSyncScheduler.Start** — Same pattern as Step 1.

**Step 3: Modify AgentCleanupService.Start** — Same pattern as Step 1.

**Step 4: Update main.go callers**

In `backend/main.go`, update the 3 call sites:

```go
// Before:
cmdbSyncScheduler.Start(1 * time.Minute)
agentCleanupService.Start(5 * time.Minute)
driftScheduler.Start(1 * time.Minute)

// After (temporary - will be moved into leader callback in Task 6):
cmdbCtx, cmdbCancel := context.WithCancel(context.Background())
defer cmdbCancel()
cmdbSyncScheduler.Start(cmdbCtx, 1*time.Minute)

cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
defer cleanupCancel()
agentCleanupService.Start(cleanupCtx, 5*time.Minute)

driftCtx, driftCancel := context.WithCancel(context.Background())
defer driftCancel()
driftScheduler.Start(driftCtx, 1*time.Minute)
```

**Step 5: Verify it compiles and runs**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./...`
Expected: No errors

**Step 6: Commit**

```bash
git add backend/services/drift_check_scheduler.go backend/services/cmdb_sync_scheduler.go backend/services/agent_cleanup_service.go backend/main.go
git commit -m "feat(ha): add context.Context to all scheduler Start methods"
```

---

## Task 6: Integrate leader election into main.go

**Files:**
- Modify: `backend/main.go` — Move all leader-only goroutines into leader election callback

**Context:** This is the core integration task. All 8 background goroutines move into the `OnStartedLeading` callback. The `RunWithFallback` function ensures local dev still works (falls back to standalone leader if K8s is not available).

**Step 1: Restructure main.go background goroutine startup**

The key change is: instead of starting all goroutines directly, wrap the leader-only ones in the leader election callback. Non-leader services (API, WebSocket Hub, C&C) continue to start unconditionally.

```go
// In main.go, after all services are initialized but before HTTP server start:

// Leader-only background services
leaderelection.RunWithFallback(ctx, leaderelection.LeaderCallbacks{
    OnStartedLeading: func(leaderCtx context.Context) {
        log.Println("[Main] This instance is now the leader, starting background services...")

        // 1. Drift check scheduler
        driftScheduler.Start(leaderCtx, 1*time.Minute)

        // 2. K8s AutoScaler
        go k8sDeploymentService.StartAutoScaler(leaderCtx, 5*time.Second)

        // 3. Pending tasks monitor
        go queueManager.StartPendingTasksMonitor(leaderCtx, 10*time.Second)

        // 4. CMDB sync
        cmdbSyncScheduler.Start(leaderCtx, 1*time.Minute)

        // 5. Agent cleanup
        agentCleanupService.Start(leaderCtx, 5*time.Minute)

        // 6. Run task timeout checker
        go runTaskTimeoutChecker.Start(leaderCtx)

        // 7. Embedding worker
        if embeddingWorker != nil {
            go embeddingWorker.Start(leaderCtx)
        }

        // 8. Lock/draft cleanup
        go func() {
            ticker := time.NewTicker(1 * time.Minute)
            defer ticker.Stop()
            for {
                select {
                case <-leaderCtx.Done():
                    log.Println("[Main] Lock/draft cleanup stopped: no longer leader")
                    return
                case <-ticker.C:
                    // existing cleanup logic
                }
            }
        }()

        // 9. Recover pending tasks (one-time on becoming leader)
        queueManager.RecoverPendingTasks()
    },
    OnStoppedLeading: func() {
        log.Println("[Main] This instance lost leadership, background services will stop via context cancellation")
    },
})
```

**Step 2: Remove the old direct goroutine starts for those 8 services**

Delete the old `go ...Start()` calls that were previously at the top level of main.go.

**Step 3: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./...`
Expected: No errors

**Step 4: Commit**

```bash
git add backend/main.go
git commit -m "feat(ha): integrate K8s leader election into main.go"
```

---

## Task 7: Replace sync.Map with PG advisory lock in TaskQueueManager

**Files:**
- Modify: `backend/services/task_queue_manager.go`

**Context:** The `workspaceLocks sync.Map` at line 23 provides per-workspace mutex to prevent concurrent task execution. This is in-memory and per-process. Replace with PG advisory lock for cross-replica safety.

**Step 1: Add pglock dependency to TaskQueueManager**

```go
import "iac-platform/backend/pkg/pglock"

type TaskQueueManager struct {
    db               *gorm.DB
    executor         *TerraformExecutor
    k8sJobService    *K8sJobService
    k8sDeploymentSvc *K8sDeploymentService
    agentCCHandler   AgentCCHandler
    pgLocker         *pglock.Locker  // replaces workspaceLocks sync.Map
}
```

**Step 2: Replace sync.Map usage**

Find all `workspaceLocks.LoadOrStore` / mutex lock/unlock patterns (around line 236) and replace:

```go
// Before (line ~236):
lock, _ := m.workspaceLocks.LoadOrStore(lockKey, &sync.Mutex{})
mutex := lock.(*sync.Mutex)
mutex.Lock()
defer mutex.Unlock()

// After:
locked, err := m.pgLocker.TryLock(int64(workspaceIDUint))
if err != nil {
    log.Printf("Failed to acquire advisory lock for workspace %s: %v", workspaceID, err)
    return
}
if !locked {
    log.Printf("Workspace %s is locked by another replica, skipping", workspaceID)
    return
}
defer m.pgLocker.Unlock(int64(workspaceIDUint))
```

**Step 3: Initialize pgLocker in NewTaskQueueManager**

```go
func NewTaskQueueManager(db *gorm.DB, executor *TerraformExecutor) *TaskQueueManager {
    return &TaskQueueManager{
        db:       db,
        executor: executor,
        pgLocker: pglock.New(db),
    }
}
```

**Step 4: Remove sync.Map field and import**

Remove `workspaceLocks sync.Map` field and clean up unused sync import if applicable.

**Step 5: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./...`
Expected: No errors

**Step 6: Commit**

```bash
git add backend/services/task_queue_manager.go
git commit -m "feat(ha): replace sync.Map with PG advisory lock in TaskQueueManager"
```

---

## Task 8: Add PG NOTIFY/LISTEN to WebSocket Hub

**Files:**
- Modify: `backend/internal/websocket/hub.go`
- Modify: `backend/main.go` — Initialize PubSub and wire into Hub

**Context:** Add `SendToSessionOrBroadcast()` method that first tries local delivery, then falls back to PG NOTIFY for cross-replica delivery. All replicas listen for `ws_broadcast` channel and deliver locally.

**Step 1: Add PubSub dependency to Hub**

In `backend/internal/websocket/hub.go`:

```go
import "iac-platform/backend/pkg/pgpubsub"

type Hub struct {
    clients    map[string]*Client
    broadcast  chan Message
    register   chan *Client
    unregister chan *Client
    mu         sync.RWMutex
    pubsub     *pgpubsub.PubSub  // NEW
    db         *gorm.DB           // NEW - for NOTIFY
    podName    string             // NEW
}
```

**Step 2: Add cross-replica broadcast message type**

```go
type CrossReplicaMessage struct {
    TargetType string `json:"target_type"` // "session"
    TargetID   string `json:"target_id"`   // session_id
    EventType  string `json:"event_type"`
    Payload    string `json:"payload"`
    SourcePod  string `json:"source_pod"`
}

const WSBroadcastChannel = "ws_broadcast"
```

**Step 3: Add SendToSessionOrBroadcast method**

```go
func (h *Hub) SendToSessionOrBroadcast(sessionID string, message Message) {
    // Try local first
    h.mu.RLock()
    client, exists := h.clients[sessionID]
    h.mu.RUnlock()

    if exists {
        h.sendToClient(client, message)
        return
    }

    // Not local — broadcast via PG NOTIFY
    if h.pubsub == nil || h.db == nil {
        log.Printf("[Hub] Session %s not local and PubSub not configured, message dropped: type=%s", sessionID, message.Type)
        return
    }

    payloadBytes, _ := json.Marshal(message.Data)
    msg := CrossReplicaMessage{
        TargetType: "session",
        TargetID:   sessionID,
        EventType:  message.Type,
        Payload:    string(payloadBytes),
        SourcePod:  h.podName,
    }
    if err := h.pubsub.Notify(h.db, WSBroadcastChannel, msg); err != nil {
        log.Printf("[Hub] Failed to broadcast via PG NOTIFY: %v", err)
    }
}
```

**Step 4: Register listener for incoming cross-replica messages**

```go
func (h *Hub) SetupCrossReplicaListener(pubsub *pgpubsub.PubSub, db *gorm.DB) {
    h.pubsub = pubsub
    h.db = db
    h.podName = os.Getenv("POD_NAME")
    if h.podName == "" {
        h.podName, _ = os.Hostname()
    }

    pubsub.Subscribe(WSBroadcastChannel, func(payload string) {
        var msg CrossReplicaMessage
        if err := json.Unmarshal([]byte(payload), &msg); err != nil {
            log.Printf("[Hub] Failed to unmarshal cross-replica message: %v", err)
            return
        }

        // Skip messages from self
        if msg.SourcePod == h.podName {
            return
        }

        if msg.TargetType == "session" {
            h.mu.RLock()
            client, exists := h.clients[msg.TargetID]
            h.mu.RUnlock()

            if exists {
                var data interface{}
                json.Unmarshal([]byte(msg.Payload), &data)
                h.sendToClient(client, Message{
                    Type:      msg.EventType,
                    SessionID: msg.TargetID,
                    Data:      data,
                })
            }
        }
    })
}
```

**Step 5: Wire up in main.go**

```go
// In main.go, after wsHub and db are initialized:
dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
    cfg.Database.Host, cfg.Database.User, cfg.Database.Password, cfg.Database.Name, cfg.Database.Port)
pubsub := pgpubsub.New(dsn)
wsHub.SetupCrossReplicaListener(pubsub, db)
if err := pubsub.Start(); err != nil {
    log.Fatalf("Failed to start PG PubSub: %v", err)
}
defer pubsub.Stop()
```

**Step 6: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./...`
Expected: No errors

**Step 7: Commit**

```bash
git add backend/internal/websocket/hub.go backend/main.go
git commit -m "feat(ha): add cross-replica WebSocket broadcast via PG NOTIFY/LISTEN"
```

---

## Task 9: Update Takeover Handler to use cross-replica broadcast

**Files:**
- Modify: `backend/internal/handlers/takeover_handler.go`

**Context:** There are 4 `wsHub.SendToSession()` calls in the takeover handler. Replace all with `wsHub.SendToSessionOrBroadcast()`.

**Step 1: Replace all 4 SendToSession calls**

The calls are at approximately these locations:

1. **RequestTakeover** (~line 87): `h.wsHub.SendToSession(...)` → `h.wsHub.SendToSessionOrBroadcast(...)`
2. **RespondToTakeover** (~line 147): Same replacement
3. **GetRequestStatus** (~line 228): Same replacement
4. **ForceTakeover** (~line 289): Same replacement

This is a straightforward find-and-replace within the file:

```
h.wsHub.SendToSession(  →  h.wsHub.SendToSessionOrBroadcast(
```

**Step 2: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./...`
Expected: No errors

**Step 3: Commit**

```bash
git add backend/internal/handlers/takeover_handler.go
git commit -m "feat(ha): use cross-replica broadcast for takeover notifications"
```

---

## Task 10: Add connected_pod tracking to Agent C&C heartbeat

**Files:**
- Modify: `backend/internal/handlers/agent_cc_handler_raw.go`

**Context:** When an agent connects or sends a heartbeat, update the `connected_pod` field in the database so that task dispatch knows which replica holds the agent's connection.

**Step 1: Update agent heartbeat handler**

Find the heartbeat processing logic in `agent_cc_handler_raw.go` and add:

```go
podName := os.Getenv("POD_NAME")
if podName == "" {
    podName, _ = os.Hostname()
}

// Update connected_pod on heartbeat
h.db.Model(&models.Agent{}).
    Where("agent_id = ?", agentID).
    Update("connected_pod", podName)
```

**Step 2: Clear connected_pod on disconnect**

In the agent disconnect/cleanup handler:

```go
h.db.Model(&models.Agent{}).
    Where("agent_id = ?", agentID).
    Update("connected_pod", nil)
```

**Step 3: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./...`
Expected: No errors

**Step 4: Commit**

```bash
git add backend/internal/handlers/agent_cc_handler_raw.go
git commit -m "feat(ha): track connected_pod in agent heartbeat"
```

---

## Task 11: Add cross-replica task dispatch via PG NOTIFY

**Files:**
- Modify: `backend/internal/handlers/agent_cc_handler_raw.go` — Add LISTEN handler for task dispatch
- Modify: `backend/services/task_queue_manager.go` — Add NOTIFY when local agent not found

**Context:** When TaskQueueManager tries to send a task to an agent but the agent is not connected to the current replica, broadcast via PG NOTIFY so the replica holding the agent's connection can dispatch it.

**Step 1: Define task dispatch channel and message**

In `agent_cc_handler_raw.go`:

```go
const TaskDispatchChannel = "task_dispatch"

type TaskDispatchMessage struct {
    AgentID     string `json:"agent_id"`
    TaskID      uint   `json:"task_id"`
    WorkspaceID string `json:"workspace_id"`
    Action      string `json:"action"`
    SourcePod   string `json:"source_pod"`
}
```

**Step 2: Add LISTEN handler to RawAgentCCHandler**

```go
func (h *RawAgentCCHandler) SetupTaskDispatchListener(pubsub *pgpubsub.PubSub) {
    podName := os.Getenv("POD_NAME")
    if podName == "" {
        podName, _ = os.Hostname()
    }

    pubsub.Subscribe(TaskDispatchChannel, func(payload string) {
        var msg TaskDispatchMessage
        if err := json.Unmarshal([]byte(payload), &msg); err != nil {
            log.Printf("[AgentCC] Failed to unmarshal task dispatch: %v", err)
            return
        }
        if msg.SourcePod == podName {
            return // Skip self
        }

        // Check if we have this agent locally
        h.mu.RLock()
        _, exists := h.agents[msg.AgentID]
        h.mu.RUnlock()

        if exists {
            if err := h.SendTaskToAgent(msg.AgentID, msg.TaskID, msg.WorkspaceID, msg.Action); err != nil {
                log.Printf("[AgentCC] Cross-replica dispatch to agent %s failed: %v", msg.AgentID, err)
            } else {
                log.Printf("[AgentCC] Cross-replica dispatch: task %d → agent %s", msg.TaskID, msg.AgentID)
            }
        }
    })
}
```

**Step 3: Update TaskQueueManager.pushTaskToAgent to use NOTIFY fallback**

In `task_queue_manager.go`, modify `pushTaskToAgent` (around line 383):

```go
// After existing SendTaskToAgent call fails with "agent not connected":
if err != nil && strings.Contains(err.Error(), "not connected") {
    // Agent might be on another replica - try PG NOTIFY
    if m.pubsub != nil && m.db != nil {
        msg := TaskDispatchMessage{
            AgentID:     agentID,
            TaskID:      task.ID,
            WorkspaceID: workspace.WorkspaceID,
            Action:      string(task.TaskType),
            SourcePod:   os.Getenv("POD_NAME"),
        }
        if notifyErr := m.pubsub.Notify(m.db, TaskDispatchChannel, msg); notifyErr != nil {
            log.Printf("[TaskQueue] PG NOTIFY dispatch failed: %v", notifyErr)
        }
        return nil // Don't treat as failure; PendingTasksMonitor will retry if needed
    }
}
```

**Step 4: Wire pubsub into TaskQueueManager**

Add a `SetPubSub` method:

```go
func (m *TaskQueueManager) SetPubSub(pubsub *pgpubsub.PubSub) {
    m.pubsub = pubsub
}
```

**Step 5: Wire up in main.go**

```go
// After pubsub is created:
queueManager.SetPubSub(pubsub)
rawCCHandler.SetupTaskDispatchListener(pubsub)
```

**Step 6: Verify it compiles**

Run: `cd /Users/ken/go/src/iac-platform/backend && go build ./...`
Expected: No errors

**Step 7: Commit**

```bash
git add backend/internal/handlers/agent_cc_handler_raw.go backend/services/task_queue_manager.go backend/main.go
git commit -m "feat(ha): add cross-replica agent task dispatch via PG NOTIFY"
```

---

## Task 12: Create K8s RBAC manifests

**Files:**
- Create: `backend/manifests/ha-rbac.yaml`

**Step 1: Write the RBAC manifests**

```yaml
# backend/manifests/ha-rbac.yaml
# K8s RBAC for IAC Platform leader election
# Apply with: kubectl apply -f ha-rbac.yaml -n <namespace>
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: iac-platform
  labels:
    app: iac-platform
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: iac-platform-leader-election
  labels:
    app: iac-platform
rules:
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "create", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: iac-platform-leader-election
  labels:
    app: iac-platform
subjects:
  - kind: ServiceAccount
    name: iac-platform
roleRef:
  kind: Role
  name: iac-platform-leader-election
  apiGroup: rbac.authorization.k8s.io
```

**Step 2: Commit**

```bash
git add backend/manifests/ha-rbac.yaml
git commit -m "feat(ha): add K8s RBAC manifests for leader election"
```

---

## Task 13: Update Deployment manifest with HA settings

**Files:**
- Create or modify: `backend/manifests/ha-deployment-patch.yaml`

**Step 1: Write the deployment patch**

```yaml
# backend/manifests/ha-deployment-patch.yaml
# Patch to apply on existing Deployment for HA support
# Usage: kubectl patch deployment iac-platform --patch-file ha-deployment-patch.yaml
spec:
  replicas: 2
  template:
    spec:
      serviceAccountName: iac-platform
      containers:
        - name: iac-platform
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
```

**Step 2: Commit**

```bash
git add backend/manifests/ha-deployment-patch.yaml
git commit -m "feat(ha): add deployment patch for HA (replicas, pod identity)"
```

---

## Summary of changes per phase

| Phase | Tasks | What changes |
|-------|-------|-------------|
| 1. Infrastructure | 1, 2, 3 | New packages: leaderelection, pglock, pgpubsub |
| 2. Scheduler context | 4, 5, 6 | Agent model update, context for schedulers, leader election in main.go |
| 3. Distributed lock | 7 | TaskQueueManager sync.Map → PG advisory lock |
| 4. Cross-replica comms | 8, 9, 10, 11 | WS broadcast, takeover fix, agent pod tracking, task dispatch |
| 5. K8s config | 12, 13 | RBAC manifests, deployment patch |
