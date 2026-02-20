# 系统信号处理实现方案

> **文档版本**: v1.0  
> **创建日期**: 2025-10-15  
> **状态**: 设计完成

## 📋 需求

### 核心需求
1. **关键阶段保护**: `applying`, `saving_state`, `saving_plan` 阶段需要忽略信号
2. **优雅退出**: 捕获信号，等待关键操作完成后再退出
3. **其他阶段**: `fetching`, `init`, `planning`, `post_plan` 可以立即响应信号
4. **全局处理**: 在main.go中全局注册信号处理
5. **日志记录**: 在日志中记录信号接收和处理过程

## 🏗️ 架构设计

### 1. 全局信号管理器

```go
// backend/services/signal_manager.go
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

// Reset 重置状态（用于测试）
func (sm *SignalManager) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	sm.gracefulExit = false
	sm.criticalSection = false
	sm.criticalStage = ""
}
```

### 2. TerraformExecutor集成

```go
// backend/services/terraform_executor.go

// 在TerraformExecutor中添加信号管理器引用
type TerraformExecutor struct {
	db            *gorm.DB
	streamManager *OutputStreamManager
	signalManager *SignalManager  // 新增
}

// 修改构造函数
func NewTerraformExecutor(db *gorm.DB, streamManager *OutputStreamManager) *TerraformExecutor {
	return &TerraformExecutor{
		db:            db,
		streamManager: streamManager,
		signalManager: GetSignalManager(),  // 获取全局信号管理器
	}
}

// 在关键阶段前后添加保护
func (s *TerraformExecutor) ExecuteApply(
	ctx context.Context,
	task *models.WorkspaceTask,
	workDir string,
) error {
	// ... 前置准备代码 ...
	
	// 检查是否应该退出（在进入关键区前）
	if s.signalManager.ShouldExit() {
		log.Printf("  Task %d: Cancelled by signal before apply", task.ID)
		return fmt.Errorf("task cancelled by signal before apply")
	}
	
	// ========== 关键区开始：Applying ==========
	s.signalManager.EnterCriticalSection("applying")
	
	// 执行terraform apply
	cmd := exec.CommandContext(ctx, "terraform", "apply",
		"-no-color",
		"-auto-approve",
		planFile,
	)
	// ... 执行代码 ...
	
	err := cmd.Run()
	
	// ========== 关键区结束：Applying ==========
	s.signalManager.ExitCriticalSection("applying")
	
	if err != nil {
		return fmt.Errorf("terraform apply failed: %w", err)
	}
	
	// 检查是否收到退出信号
	if s.signalManager.IsGracefulExit() {
		log.Printf("  Task %d: Signal received, but apply completed. Proceeding to save state...", task.ID)
	}
	
	// ========== 关键区开始：Saving State ==========
	s.signalManager.EnterCriticalSection("saving_state")
	
	// 保存State到数据库
	err = s.SaveNewStateVersion(workspace, task, workDir)
	
	// ========== 关键区结束：Saving State ==========
	s.signalManager.ExitCriticalSection("saving_state")
	
	if err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}
	
	// 检查是否应该退出
	if s.signalManager.ShouldExit() {
		log.Printf(" Task %d: Critical operations completed, exiting gracefully", task.ID)
		return fmt.Errorf("task cancelled by signal after critical operations completed")
	}
	
	return nil
}

// 在Plan执行中添加保护
func (s *TerraformExecutor) ExecutePlan(
	ctx context.Context,
	task *models.WorkspaceTask,
	workDir string,
) error {
	// ... 前置代码 ...
	
	// Fetching阶段 - 可以立即退出
	if s.signalManager.ShouldExit() {
		return fmt.Errorf("task cancelled by signal during fetching")
	}
	
	// Init阶段 - 可以立即退出
	if err := s.TerraformInit(ctx, workDir, task); err != nil {
		return err
	}
	
	if s.signalManager.ShouldExit() {
		return fmt.Errorf("task cancelled by signal after init")
	}
	
	// Planning阶段 - 可以立即退出
	if err := s.TerraformPlan(ctx, workDir, task); err != nil {
		return err
	}
	
	if s.signalManager.ShouldExit() {
		return fmt.Errorf("task cancelled by signal after plan")
	}
	
	// ========== 关键区开始：Saving Plan ==========
	s.signalManager.EnterCriticalSection("saving_plan")
	
	// 保存Plan数据到数据库
	err := s.SavePlanData(task, planFile, planJSON)
	
	// ========== 关键区结束：Saving Plan ==========
	s.signalManager.ExitCriticalSection("saving_plan")
	
	if err != nil {
		log.Printf("  Failed to save plan data: %v", err)
		// Plan数据保存失败不阻塞任务成功
	}
	
	// 检查是否应该退出
	if s.signalManager.ShouldExit() {
		log.Printf(" Task %d: Plan data saved, exiting gracefully", task.ID)
		return fmt.Errorf("task cancelled by signal after plan data saved")
	}
	
	return nil
}
```

### 3. Main.go集成

```go
// backend/main.go

func main() {
	// ... 现有初始化代码 ...
	
	// 初始化全局信号管理器
	signalManager := services.GetSignalManager()
	log.Println("Global signal manager initialized")
	
	// 初始化任务队列管理器
	executor := services.NewTerraformExecutor(db, streamManager)
	queueManager := services.NewTaskQueueManager(db, executor)
	log.Println("Task queue manager initialized")
	
	// ... 其他初始化代码 ...
	
	// 设置优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	
	// 启动服务器
	go func() {
		log.Printf("Server starting on port %s", port)
		if err := r.Run(":" + port); err != nil {
			log.Fatal("Failed to start server:", err)
		}
	}()
	
	// 等待退出信号
	<-quit
	log.Println("  Shutting down server...")
	
	// 等待关键操作完成
	if signalManager.IsGracefulExit() {
		log.Println("⏳ Waiting for critical operations to complete...")
		
		// 轮询检查，最多等待30秒
		for i := 0; i < 30; i++ {
			if !signalManager.IsGracefulExit() || !signalManager.criticalSection {
				break
			}
			time.Sleep(1 * time.Second)
			log.Printf("⏳ Still waiting... (%d/30s)", i+1)
		}
	}
	
	log.Println(" Server exited gracefully")
}
```

## 🔧 实施步骤

### Step 1: 创建SignalManager
创建 `backend/services/signal_manager.go`

### Step 2: 修改TerraformExecutor
在 `backend/services/terraform_executor.go` 中：
1. 添加 `signalManager` 字段
2. 在构造函数中初始化
3. 在关键阶段添加保护

### Step 3: 修改Main.go
在 `backend/main.go` 中：
1. 初始化全局信号管理器
2. 优雅关闭时等待关键操作完成

### Step 4: 添加日志
在所有关键点添加详细日志

## 📊 信号处理流程图

```
用户按Ctrl+C (SIGINT)
    ↓
SignalManager接收信号
    ↓
检查是否在关键区？
    ├─ 是 → 设置gracefulExit=true，记录日志，继续执行
    │         ↓
    │      关键操作完成
    │         ↓
    │      退出关键区
    │         ↓
    │      下一个检查点发现shouldExit()=true
    │         ↓
    │      返回错误，任务标记为cancelled
    │
    └─ 否 → 设置gracefulExit=true，记录日志
              ↓
           下一个检查点发现shouldExit()=true
              ↓
           立即返回错误，任务标记为cancelled
```

## 🧪 测试场景

### 场景1: 在Fetching阶段收到信号
-  立即退出
-  任务标记为cancelled
-  日志记录信号

### 场景2: 在Applying阶段收到信号
-  记录信号但继续执行
-  Apply完成后进入Saving State
-  Saving State完成后优雅退出
-  任务标记为cancelled（但数据已保存）

### 场景3: 在Saving State阶段收到信号
-  记录信号但继续执行
-  State保存完成后优雅退出
-  任务标记为cancelled（但State已保存）

### 场景4: 在Saving Plan阶段收到信号
-  记录信号但继续执行
-  Plan数据保存完成后优雅退出
-  任务标记为cancelled（但Plan数据已保存）

## 📝 实施检查清单

- [ ] 创建 `backend/services/signal_manager.go`
- [ ] 修改 `backend/services/terraform_executor.go`
  - [ ] 添加signalManager字段
  - [ ] 在ExecutePlan中添加保护
  - [ ] 在ExecuteApply中添加保护
  - [ ] 在SavePlanData中添加保护
  - [ ] 在SaveNewStateVersion中添加保护
- [ ] 修改 `backend/main.go`
  - [ ] 初始化全局信号管理器
  - [ ] 优雅关闭时等待关键操作
- [ ] 添加单元测试
- [ ] 添加集成测试
- [ ] 文档更新

## 🔗 相关文档

- [15-terraform-execution-detail.md](./15-terraform-execution-detail.md) - Terraform执行流程
- [04-task-workflow.md](./04-task-workflow.md) - 任务工作流

---

**状态**: 设计完成，待实施
