package controllers

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// checkWebSocketOrigin 验证 WebSocket 连接的来源
// 通过 ALLOWED_ORIGINS 环境变量配置白名单（逗号分隔），未设置则允许所有来源
func checkWebSocketOrigin(r *http.Request) bool {
	allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")
	if allowedOriginsEnv == "" {
		return true // 未配置白名单时允许所有（开发环境兼容）
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // 无 Origin header（非浏览器客户端）
	}

	for _, allowed := range strings.Split(allowedOriginsEnv, ",") {
		allowed = strings.TrimSpace(allowed)
		if allowed == origin {
			return true
		}
	}

	log.Printf("[WebSocket] Blocked connection from origin: %s", origin)
	return false
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkWebSocketOrigin,
	Subprotocols:    []string{"access_token"}, // 支持 access_token 子协议
}

// TerraformOutputController WebSocket输出控制器
type TerraformOutputController struct {
	streamManager *services.OutputStreamManager
	db            *gorm.DB
	iam           *middleware.IAMPermissionMiddleware
}

// NewTerraformOutputController 创建控制器
func NewTerraformOutputController(streamManager *services.OutputStreamManager, db *gorm.DB, iam *middleware.IAMPermissionMiddleware) *TerraformOutputController {
	return &TerraformOutputController{
		streamManager: streamManager,
		db:            db,
		iam:           iam,
	}
}

// StreamTaskOutput WebSocket实时输出
// @Summary WebSocket实时日志流
// @Description 通过WebSocket实时获取任务执行日志
// @Tags Task Log
// @Accept json
// @Produce json
// @Param task_id path int true "任务ID"
// @Success 101 {object} map[string]interface{} "WebSocket连接建立"
// @Failure 400 {object} map[string]interface{} "无效的任务ID"
// @Router /api/v1/tasks/{task_id}/output/stream [get]
// @Security BearerAuth
func (c *TerraformOutputController) StreamTaskOutput(ctx *gin.Context) {
	taskIDStr := ctx.Param("task_id")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		ctx.JSON(400, gin.H{"error": "invalid task_id"})
		return
	}

	// 完整的 task → workspace → project → org 绑定以及 IAM 校验必须在
	// Upgrade 之前完成；db/IAM 缺失或歧义绑定一律 fail-closed。
	if _, ok := loadAndAuthorizeTaskWorkspace(ctx, c.db, c.iam); !ok {
		return
	}

	// 升级到WebSocket
	ws, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer ws.Close()

	// 生成客户端ID
	clientID := uuid.New().String()
	log.Printf("Client %s connecting to task %d", clientID, taskID)

	// 获取输出流
	stream := c.streamManager.Get(uint(taskID))
	if stream == nil {
		// 任务可能还没开始或已完成，尝试创建流
		stream = c.streamManager.GetOrCreate(uint(taskID))
	}

	// 订阅输出流（同时获取历史消息）
	client, history := stream.Subscribe(clientID)
	if client == nil {
		ws.WriteJSON(map[string]string{
			"type":  "error",
			"error": "failed to subscribe to stream",
		})
		return
	}
	defer stream.Unsubscribe(clientID)

	// 发送连接成功消息
	ws.WriteJSON(map[string]interface{}{
		"type":      "connected",
		"task_id":   taskID,
		"client_id": clientID,
	})

	// 发送历史消息
	for _, msg := range history {
		if err := ws.WriteJSON(msg); err != nil {
			log.Printf("Failed to send history: %v", err)
			return
		}
	}

	// 设置心跳
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 实时转发新消息
	for {
		select {
		case msg, ok := <-client.Channel:
			if !ok {
				// 通道关闭，任务完成
				return
			}

			if err := ws.WriteJSON(msg); err != nil {
				log.Printf("WebSocket write failed: %v", err)
				return
			}

		case <-ticker.C:
			// 发送心跳
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Ping failed: %v", err)
				return
			}
		}
	}
}

// GetStreamStats 获取流统计信息（调试用）
// @Summary 获取输出流统计
// @Description 获取所有输出流的统计信息（调试用）
// @Tags Task Log
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "成功返回统计信息"
// @Router /api/v1/terraform/streams/stats [get]
// @Security BearerAuth
func (c *TerraformOutputController) GetStreamStats(ctx *gin.Context) {
	stats := c.streamManager.GetAllStats()
	ctx.JSON(200, gin.H{
		"streams": stats,
		"count":   len(stats),
	})
}
