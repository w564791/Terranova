package services

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestSignalManager_Singleton(t *testing.T) {
	// 测试单例模式
	sm1 := GetSignalManager()
	sm2 := GetSignalManager()

	if sm1 != sm2 {
		t.Error("SignalManager should be singleton")
	}
}

func TestSignalManager_CriticalSection(t *testing.T) {
	sm := GetSignalManager()
	sm.Reset() // 重置状态

	// 初始状态
	if sm.IsCriticalSection() {
		t.Error("Should not be in critical section initially")
	}

	// 进入关键区
	sm.EnterCriticalSection("test_stage")
	if !sm.IsCriticalSection() {
		t.Error("Should be in critical section after Enter")
	}

	// 退出关键区
	sm.ExitCriticalSection("test_stage")
	if sm.IsCriticalSection() {
		t.Error("Should not be in critical section after Exit")
	}
}

func TestSignalManager_GracefulExit(t *testing.T) {
	sm := GetSignalManager()
	sm.Reset()

	// 初始状态
	if sm.IsGracefulExit() {
		t.Error("Should not have graceful exit flag initially")
	}

	if sm.ShouldExit() {
		t.Error("Should not exit initially")
	}

	// 模拟收到信号（非关键区）
	sm.gracefulExit = true

	if !sm.IsGracefulExit() {
		t.Error("Should have graceful exit flag after signal")
	}

	if !sm.ShouldExit() {
		t.Error("Should exit when not in critical section")
	}
}

func TestSignalManager_CriticalSectionProtection(t *testing.T) {
	sm := GetSignalManager()
	sm.Reset()

	// 进入关键区
	sm.EnterCriticalSection("applying")

	// 模拟收到信号
	sm.gracefulExit = true

	// 在关键区内不应该退出
	if sm.ShouldExit() {
		t.Error("Should not exit while in critical section")
	}

	// 退出关键区后应该退出
	sm.ExitCriticalSection("applying")
	if !sm.ShouldExit() {
		t.Error("Should exit after leaving critical section")
	}
}

func TestSignalManager_MultipleEnterExit(t *testing.T) {
	sm := GetSignalManager()
	sm.Reset()

	// 多次进入退出
	sm.EnterCriticalSection("stage1")
	sm.ExitCriticalSection("stage1")

	sm.EnterCriticalSection("stage2")
	if !sm.IsCriticalSection() {
		t.Error("Should be in critical section")
	}
	sm.ExitCriticalSection("stage2")

	if sm.IsCriticalSection() {
		t.Error("Should not be in critical section after all exits")
	}
}

func TestSignalManager_Reset(t *testing.T) {
	sm := GetSignalManager()

	// 设置一些状态
	sm.EnterCriticalSection("test")
	sm.gracefulExit = true

	// 重置
	sm.Reset()

	// 验证状态已重置
	if sm.IsCriticalSection() {
		t.Error("Should not be in critical section after reset")
	}
	if sm.IsGracefulExit() {
		t.Error("Should not have graceful exit flag after reset")
	}
	if sm.ShouldExit() {
		t.Error("Should not exit after reset")
	}
}

// 集成测试：模拟实际信号处理流程
func TestSignalManager_IntegrationFlow(t *testing.T) {
	sm := GetSignalManager()
	sm.Reset()

	// 模拟Plan执行流程
	t.Run("Plan execution with signal", func(t *testing.T) {
		// 阶段1: Fetching（非关键区）
		if sm.ShouldExit() {
			t.Error("Should not exit at start")
		}

		// 阶段2: Init（非关键区）
		// 模拟收到信号
		sm.gracefulExit = true

		// 应该可以立即退出
		if !sm.ShouldExit() {
			t.Error("Should exit in non-critical section")
		}

		// 重置测试
		sm.Reset()
	})

	t.Run("Apply execution with signal in critical section", func(t *testing.T) {
		// 阶段1: Fetching（非关键区）
		// 阶段2: Init（非关键区）
		// 阶段3: Restoring Plan（非关键区）

		// 阶段4: Applying（关键区）
		sm.EnterCriticalSection("applying")

		// 模拟收到信号
		sm.gracefulExit = true

		// 在关键区内不应该退出
		if sm.ShouldExit() {
			t.Error("Should not exit during applying")
		}

		// Apply完成，退出关键区
		sm.ExitCriticalSection("applying")

		// 阶段5: Saving State（关键区）
		sm.EnterCriticalSection("saving_state")

		// 仍然不应该退出
		if sm.ShouldExit() {
			t.Error("Should not exit during saving_state")
		}

		// State保存完成
		sm.ExitCriticalSection("saving_state")

		// 现在应该可以退出
		if !sm.ShouldExit() {
			t.Error("Should exit after all critical sections completed")
		}

		sm.Reset()
	})
}

// 性能测试：并发访问
func TestSignalManager_Concurrency(t *testing.T) {
	sm := GetSignalManager()
	sm.Reset()

	done := make(chan bool)
	iterations := 1000

	// 并发进入/退出关键区
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < iterations; j++ {
				sm.EnterCriticalSection("concurrent_test")
				time.Sleep(time.Microsecond)
				sm.ExitCriticalSection("concurrent_test")
			}
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证最终状态
	if sm.IsCriticalSection() {
		t.Error("Should not be in critical section after all goroutines complete")
	}
}

// 实际信号测试（需要手动运行）
func TestSignalManager_ActualSignal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping actual signal test in short mode")
	}

	sm := GetSignalManager()
	sm.Reset()

	// 发送SIGINT信号给自己
	go func() {
		time.Sleep(100 * time.Millisecond)
		syscall.Kill(os.Getpid(), syscall.SIGINT)
	}()

	// 等待信号被处理
	time.Sleep(200 * time.Millisecond)

	// 验证信号被接收
	if !sm.IsGracefulExit() {
		t.Error("Should have graceful exit flag after SIGINT")
	}

	sm.Reset()
}
