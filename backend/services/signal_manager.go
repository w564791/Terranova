package services

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// SignalManager 全局信号管理器
type SignalManager struct {
	shutdownChan    chan os.Signal
	gracefulExit    bool
	criticalSection bool
	criticalStage   string
	mu              sync.RWMutex
}

var (
	globalSignalManager *SignalManager
	once                sync.Once
)

// GetSignalManager 获取全局信号管理器（单例）
func GetSignalManager() *SignalManager {
	once.Do(func() {
		globalSignalManager = &SignalManager{
			shutdownChan:    make(chan os.Signal, 1),
			gracefulExit:    false,
			criticalSection: false,
		}

		// 注册信号
		signal.Notify(globalSignalManager.shutdownChan, syscall.SIGINT, syscall.SIGTERM)

		// 启动信号监听
		go globalSignalManager.handleSignals()

		log.Println("Global signal manager initialized")
	})

	return globalSignalManager
}

// handleSignals 处理接收到的信号
func (sm *SignalManager) handleSignals() {
	for sig := range sm.shutdownChan {
		sm.mu.Lock()

		log.Printf("Received signal: %v", sig)

		if sm.criticalSection {
			log.Printf("🔒 In critical section [%s], will exit gracefully after completion", sm.criticalStage)
			sm.gracefulExit = true
		} else {
			log.Printf("Not in critical section, can exit immediately")
			sm.gracefulExit = true
		}

		sm.mu.Unlock()
	}
}

// EnterCriticalSection 进入关键区
func (sm *SignalManager) EnterCriticalSection(stage string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.criticalSection = true
	sm.criticalStage = stage
	log.Printf("🔒 Entered critical section: %s", stage)
}

// ExitCriticalSection 退出关键区
func (sm *SignalManager) ExitCriticalSection(stage string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.criticalSection = false
	sm.criticalStage = ""
	log.Printf("🔓 Exited critical section: %s", stage)

	// 如果有待处理的退出信号，记录日志
	if sm.gracefulExit {
		log.Printf(" Critical section completed, ready for graceful exit")
	}
}

// ShouldExit 检查是否应该退出
func (sm *SignalManager) ShouldExit() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 只有在非关键区且收到退出信号时才返回true
	return sm.gracefulExit && !sm.criticalSection
}

// IsGracefulExit 检查是否收到退出信号
func (sm *SignalManager) IsGracefulExit() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.gracefulExit
}

// IsCriticalSection 检查是否在关键区
func (sm *SignalManager) IsCriticalSection() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.criticalSection
}

// Reset 重置状态（用于测试）
func (sm *SignalManager) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.gracefulExit = false
	sm.criticalSection = false
	sm.criticalStage = ""
	log.Println("🔄 Signal manager reset")
}
