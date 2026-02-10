package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"iac-platform/internal/config"
	"iac-platform/internal/database"
	"iac-platform/internal/handlers"
	"iac-platform/internal/models"
	"iac-platform/internal/router"
	"iac-platform/internal/websocket"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
)

// @title IAC Platform API
// @version 1.0
// @description Infrastructure as Code Platform API Documentation
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@iac-platform.com

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /
// @schemes http https

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

func main() {
	// 设置时区为本地时区
	loc, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		log.Printf("Warning: Failed to load timezone, using system default: %v", err)
	} else {
		time.Local = loc
		log.Println("Timezone set to Asia/Singapore (UTC+8)")
	}

	// 加载配置
	cfg := config.Load()

	// 初始化数据库
	db, err := database.Initialize(cfg.Database)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// 初始化全局信号管理器
	signalManager := services.GetSignalManager()
	log.Println("Global signal manager initialized")

	// 初始化输出流管理器
	streamManager := services.NewOutputStreamManager()
	streamManager.StartCleanupWorker()
	log.Println("Output stream manager initialized")

	// 初始化任务队列管理器
	executor := services.NewTerraformExecutor(db, streamManager)
	queueManager := services.NewTaskQueueManager(db, executor)
	log.Println("Task queue manager initialized")

	// 设置全局 TaskQueueManager（用于 Drift Check 手动触发）
	services.SetGlobalTaskQueueManager(queueManager)
	log.Println("Global TaskQueueManager set for drift check")

	// 初始化平台配置服务
	// 从数据库 system_configs 表读取配置，支持动态修改
	platformConfigService := services.NewPlatformConfigService(db)
	baseURL := platformConfigService.GetBaseURL()
	log.Printf("Platform base URL: %s (from database or environment)", baseURL)

	// 初始化 Run Task 执行器
	runTaskExecutor := services.NewRunTaskExecutor(db, baseURL)
	executor.SetRunTaskExecutor(runTaskExecutor)
	log.Println("Run Task executor initialized and configured")

	// 启动 Run Task 超时检查器
	runTaskTimeoutChecker := services.NewRunTaskTimeoutChecker(db, 30*time.Second)
	runTaskTimeoutCtx, runTaskTimeoutCancel := context.WithCancel(context.Background())
	defer runTaskTimeoutCancel()
	go runTaskTimeoutChecker.Start(runTaskTimeoutCtx)
	log.Println("Run Task timeout checker started (30 second interval)")

	// 初始化资源编辑协作服务
	editingService := services.NewResourceEditingService(db)
	log.Println("Resource editing service initialized")

	// 初始化WebSocket Hub
	wsHub := websocket.NewHub()
	go wsHub.Run()
	log.Println("WebSocket Hub initialized and running")

	// 初始化Agent Metrics Hub
	agentMetricsHub := websocket.NewAgentMetricsHub()
	go agentMetricsHub.Run()
	log.Println("Agent Metrics Hub initialized and running")

	// 初始化CMDB外部数据源同步调度器
	cmdbSyncScheduler := services.NewCMDBSyncScheduler(db)
	cmdbSyncScheduler.Start(1 * time.Minute) // 每分钟检查一次需要同步的数据源
	log.Println("CMDB external source sync scheduler started (1 minute check interval)")

	// 初始化Agent清理服务
	agentCleanupService := services.NewAgentCleanupService(db)
	agentCleanupService.Start(5 * time.Minute)
	log.Println("Agent cleanup service started (5 minute interval)")

	// 初始化K8s Deployment服务
	k8sDeploymentService, err := services.NewK8sDeploymentService(db)
	if err != nil {
		log.Printf("Warning: Failed to initialize K8s Deployment service: %v", err)
		log.Println("K8s agent pools will not be available")
	} else {
		log.Println("K8s Deployment service initialized")

		// 为所有K8s pools创建deployments
		go func() {
			ctx := context.Background()
			var pools []struct {
				PoolID   string
				Name     string
				PoolType string
			}

			if err := db.Table("agent_pools").
				Where("pool_type = ?", "k8s").
				Select("pool_id, name, pool_type").
				Find(&pools).Error; err != nil {
				log.Printf("[K8sDeployment] Error fetching K8s pools: %v", err)
				return
			}

			log.Printf("[K8sDeployment] Found %d active K8s pools, ensuring deployments exist", len(pools))

			for _, poolInfo := range pools {
				// Fetch full pool data
				var pool models.AgentPool
				if err := db.Where("pool_id = ?", poolInfo.PoolID).First(&pool).Error; err != nil {
					log.Printf("[K8sDeployment] Error fetching pool %s: %v", poolInfo.PoolID, err)
					continue
				}

				// Ensure deployment exists for this pool
				if err := k8sDeploymentService.EnsureDeploymentForPool(ctx, &pool); err != nil {
					log.Printf("[K8sDeployment] Error ensuring deployment for pool %s: %v", poolInfo.PoolID, err)
				} else {
					log.Printf("[K8sDeployment] Deployment ensured for pool %s", poolInfo.PoolID)
				}
			}

			log.Println("[K8sDeployment] All K8s pool deployments initialized")
		}()

		// 启动auto-scaler goroutine (每30秒检查一次)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go k8sDeploymentService.StartAutoScaler(ctx, 5*time.Second)
		log.Println("K8s Deployment auto-scaler started (5 second interval)")
	}

	// 启动后台清理任务
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		log.Println("Background cleanup worker started (1 minute interval)")

		for range ticker.C {
			// 清理过期的资源锁（2分钟无心跳）
			if err := editingService.CleanupExpiredLocks(); err != nil {
				log.Printf("Error cleaning up expired locks: %v", err)
			}

			// 清理旧的草稿（7天前的expired状态）
			if err := editingService.CleanupOldDrifts(); err != nil {
				log.Printf("Error cleaning up old drifts: %v", err)
			}

			// 清理过期的接管请求
			if err := editingService.CleanupExpiredRequests(); err != nil {
				log.Printf("Error cleaning up expired takeover requests: %v", err)
			}
		}
	}()

	// 设置Gin模式
	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		// 开发环境也禁用Gin的默认日志，减少日志噪音
		gin.SetMode(gin.ReleaseMode)
	}

	// 启动独立的WebSocket服务器来处理Agent连接
	// 这避免了Gin框架对WebSocket的干扰
	rawCCHandler := handlers.NewRawAgentCCHandler(db, streamManager)
	rawCCHandler.SetMetricsHub(agentMetricsHub)
	log.Println("Agent C&C handler (Raw WebSocket) initialized with stream manager and metrics hub")

	// 获取WebSocket服务器端口
	// 优先使用 CC_SERVER_PORT，如果未设置则使用 SERVER_PORT + 10
	wsPort := os.Getenv("CC_SERVER_PORT")
	if wsPort == "" {
		// 获取API服务器端口
		apiPort := os.Getenv("SERVER_PORT")
		if apiPort == "" {
			apiPort = "8080"
		}

		// 计算WebSocket端口（API端口 + 10）
		var apiPortNum int
		if _, err := fmt.Sscanf(apiPort, "%d", &apiPortNum); err != nil {
			log.Fatalf("[AgentWS] Invalid SERVER_PORT: %v", err)
		}

		wsPortNum := apiPortNum + 10
		wsPort = fmt.Sprintf("%d", wsPortNum)
		log.Printf("[AgentWS] CC_SERVER_PORT not set, using SERVER_PORT + 10: %s", wsPort)
	} else {
		log.Printf("[AgentWS] Using configured CC_SERVER_PORT: %s", wsPort)
	}

	log.Printf("[AgentWS] Starting standalone WebSocket server on port %s", wsPort)

	// 使用channel来确保WebSocket服务器启动成功
	wsReady := make(chan error, 1)

	go func() {
		if err := rawCCHandler.StartStandalone(wsPort); err != nil {
			wsReady <- err
		}
	}()

	// 给WebSocket服务器一点时间启动，但不要阻塞太久
	// 使用非阻塞的select来快速检查是否有错误
	select {
	case err := <-wsReady:
		log.Fatalf("[AgentWS] Failed to start: %v", err)
	case <-time.After(100 * time.Millisecond):
		// 100ms后如果没有错误，认为启动成功
		log.Printf("[AgentWS] WebSocket server started successfully")
	}

	// 将Agent C&C Handler注入到TaskQueueManager（必须在 RecoverPendingTasks 之前）
	// 使用rawCCHandler因为这是实际处理agent连接的handler
	queueManager.SetAgentCCHandler(rawCCHandler)
	log.Println("[TaskQueue] Agent C&C handler configured (using Raw WebSocket handler)")

	// 【Phase 2优化】将K8s Deployment Service注入到TaskQueueManager（用于槽位管理）
	if k8sDeploymentService != nil {
		queueManager.SetK8sDeploymentService(k8sDeploymentService)
		log.Println("[TaskQueue] K8s Deployment Service configured for slot management")
	}

	// 注册C&C handler到CCNotifier（用于credentials refresh通知）
	ccNotifier := services.GetCCNotifier()
	ccNotifier.SetCCHandler(rawCCHandler)
	log.Println("[CCNotifier] C&C handler registered for credentials refresh notifications")

	// 初始化路由，传入rawCCHandler以支持任务取消功能和agentMetricsHub以支持实时metrics
	// 同时传入runTaskExecutor以支持Agent模式下的Run Task执行
	r := router.Setup(db, streamManager, wsHub, agentMetricsHub, queueManager, rawCCHandler, runTaskExecutor)

	log.Println("System initialized with task queue management and Agent C&C support")

	// 启动 Embedding Worker（CMDB 向量化搜索后台处理）
	embeddingWorker := router.GetEmbeddingWorker()
	if embeddingWorker != nil {
		embeddingWorkerCtx, embeddingWorkerCancel := context.WithCancel(context.Background())
		defer embeddingWorkerCancel()
		go embeddingWorker.Start(embeddingWorkerCtx)
		log.Println("Embedding worker started for CMDB vector search")
	}

	// 恢复pending任务（必须在 AgentCCHandler 初始化之后）
	if err := queueManager.RecoverPendingTasks(); err != nil {
		log.Printf("Warning: Failed to recover pending tasks: %v", err)
	}

	// 启动pending任务监控器（10秒检查一次）
	// 这确保所有pending任务都能得到执行机会,即使之前的尝试失败了
	monitorCtx, monitorCancel := context.WithCancel(context.Background())
	defer monitorCancel()
	go queueManager.StartPendingTasksMonitor(monitorCtx, 10*time.Second)
	log.Println("Pending tasks monitor started (10 second interval)")

	// 初始化并启动 Drift 检测调度器
	driftScheduler := services.NewDriftCheckScheduler(db, queueManager)
	driftScheduler.Start(1 * time.Minute)
	log.Println("Drift check scheduler started (1 minute check interval)")

	// 设置优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 启动服务器
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	// 获取监听地址，默认监听所有网络接口（0.0.0.0）以支持手机访问
	host := os.Getenv("SERVER_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	// 在goroutine中启动服务器
	go func() {
		addr := host + ":" + port

		// 检查 TLS 证书：优先使用环境变量，否则自动探测常见路径
		tlsCert := os.Getenv("TLS_CERT_FILE")
		tlsKey := os.Getenv("TLS_KEY_FILE")

		if tlsCert == "" || tlsKey == "" {
			// 自动探测证书路径（相对于工作目录）
			searchPaths := []struct{ cert, key string }{
				{"../certs/localhost.pem", "../certs/localhost-key.pem"},
				{"certs/localhost.pem", "certs/localhost-key.pem"},
			}
			for _, p := range searchPaths {
				certAbs, _ := filepath.Abs(p.cert)
				keyAbs, _ := filepath.Abs(p.key)
				if _, err := os.Stat(certAbs); err == nil {
					if _, err := os.Stat(keyAbs); err == nil {
						tlsCert = certAbs
						tlsKey = keyAbs
						break
					}
				}
			}
		}

		if tlsCert != "" && tlsKey != "" {
			log.Printf("🔒 Server starting on https://%s (TLS enabled)", addr)
			log.Printf("   cert: %s", tlsCert)
			log.Printf("   key:  %s", tlsKey)

			srv := &http.Server{
				Addr:    addr,
				Handler: r,
				TLSConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			}
			if err := srv.ListenAndServeTLS(tlsCert, tlsKey); err != nil && err != http.ErrServerClosed {
				log.Fatal("Failed to start TLS server:", err)
			}
		} else {
			log.Printf("Server starting on http://%s (accessible from network)", addr)
			if err := r.Run(addr); err != nil {
				log.Fatal("Failed to start server:", err)
			}
		}
	}()

	// 等待退出信号
	<-quit
	log.Println("Shutting down server...")

	// 停止CMDB同步调度器
	cmdbSyncScheduler.Stop()
	log.Println("CMDB sync scheduler stopped")

	// 停止Agent清理服务
	agentCleanupService.Stop()
	log.Println("Agent cleanup service stopped")

	// K8s Deployment auto-scaler会通过context自动停止
	log.Println("K8s Deployment auto-scaler stopped")

	// 等待关键操作完成
	if signalManager.IsCriticalSection() {
		log.Println("⏳ Waiting for critical operations to complete...")

		// 轮询检查，最多等待30秒
		for i := 0; i < 30; i++ {
			if !signalManager.IsCriticalSection() {
				log.Println("Critical operations completed")
				break
			}
			// time.Sleep(1 * time.Second)
			log.Printf("Still waiting... (%d/30s)", i+1)
		}

		if signalManager.IsCriticalSection() {
			log.Println("Timeout waiting for critical operations, forcing shutdown")
		}
	}

	log.Println("Server exited gracefully")
}
