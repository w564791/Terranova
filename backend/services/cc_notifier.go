package services

import (
	"log"
	"sync"
)

// CCNotifier provides a way to send C&C notifications to agents
// This is a singleton that holds a reference to the RawAgentCCHandler
type CCNotifier struct {
	ccHandler CCHandlerInterface
	mu        sync.RWMutex
}

// CCHandlerInterface defines the interface for C&C handler
type CCHandlerInterface interface {
	BroadcastCredentialsRefresh(poolID string) error
}

var (
	ccNotifierInstance *CCNotifier
	ccNotifierOnce     sync.Once
)

// GetCCNotifier returns the singleton CCNotifier instance
func GetCCNotifier() *CCNotifier {
	ccNotifierOnce.Do(func() {
		ccNotifierInstance = &CCNotifier{}
	})
	return ccNotifierInstance
}

// SetCCHandler sets the C&C handler instance
func (n *CCNotifier) SetCCHandler(handler CCHandlerInterface) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ccHandler = handler
	log.Printf("[CCNotifier] C&C handler registered")
}

// NotifyCredentialsRefresh sends credentials refresh notification to all agents in a pool
func (n *CCNotifier) NotifyCredentialsRefresh(poolID string) error {
	n.mu.RLock()
	handler := n.ccHandler
	n.mu.RUnlock()

	if handler == nil {
		log.Printf("[CCNotifier] C&C handler not initialized, skipping notification for pool %s", poolID)
		return nil
	}

	log.Printf("[CCNotifier] Sending credentials refresh notification to pool %s", poolID)
	return handler.BroadcastCredentialsRefresh(poolID)
}
