// Run Task Test Platform
// 用于测试 Run Task 功能的模拟第三方服务
//
// 支持三种测试场景：
// 1. /success - 立即回调成功
// 2. /failure - 立即回调失败
// 3. /timeout - 不回调（模拟超时）
//
// 启动方式：
//   go run backend/cmd/runtask-test-platform/main.go
//
// 默认端口：118090

package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// RunTaskPayload 接收的 Run Task 请求体
type RunTaskPayload struct {
	PayloadVersion             int                    `json:"payload_version"`
	Stage                      string                 `json:"stage"`
	AccessToken                string                 `json:"access_token"`
	TaskResultID               string                 `json:"task_result_id"`
	TaskResultCallbackURL      string                 `json:"task_result_callback_url"`
	TaskResultEnforcementLevel string                 `json:"task_result_enforcement_level"`
	TaskID                     int                    `json:"task_id"`
	TaskType                   string                 `json:"task_type"`
	TaskStatus                 string                 `json:"task_status"`
	TaskDescription            string                 `json:"task_description"`
	TaskCreatedAt              string                 `json:"task_created_at"`
	TaskAppURL                 string                 `json:"task_app_url"`
	WorkspaceID                string                 `json:"workspace_id"`
	TimeoutSeconds             int                    `json:"timeout_seconds"`
	Capabilities               map[string]interface{} `json:"capabilities"`
	PlanJSONAPIURL             string                 `json:"plan_json_api_url,omitempty"`
	ResourceChangesAPIURL      string                 `json:"resource_changes_api_url,omitempty"`
}

// CallbackPayload 回调请求体（JSON:API 格式）
type CallbackPayload struct {
	Data CallbackData `json:"data"`
}

type CallbackData struct {
	Type          string                 `json:"type"`
	Attributes    CallbackAttributes     `json:"attributes"`
	Relationships *CallbackRelationships `json:"relationships,omitempty"`
}

type CallbackAttributes struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	URL     string `json:"url,omitempty"`
}

type CallbackRelationships struct {
	Outcomes *OutcomesRelationship `json:"outcomes,omitempty"`
}

type OutcomesRelationship struct {
	Data []OutcomeData `json:"data"`
}

type OutcomeData struct {
	Type       string            `json:"type"`
	Attributes OutcomeAttributes `json:"attributes"`
}

type OutcomeAttributes struct {
	OutcomeID   string                 `json:"outcome-id"`
	Description string                 `json:"description"`
	Body        string                 `json:"body,omitempty"`
	URL         string                 `json:"url,omitempty"`
	Tags        map[string]interface{} `json:"tags,omitempty"`
}

var (
	port       = getEnv("PORT", "18090")
	baseURL    = getEnv("BASE_URL", "http://localhost:18090")
	hmacKey    = getEnv("HMAC_KEY", "test-hmac-secret-key")
	httpClient = &http.Client{Timeout: 30 * time.Second}
)

func main() {
	// 设置路由
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/test", handleTest)

	// 成功端点及其测试
	http.HandleFunc("/success", handleSuccess)
	http.HandleFunc("/success/test", handleTest)

	// 失败端点及其测试
	http.HandleFunc("/failure", handleFailure)
	http.HandleFunc("/failure/test", handleTest)

	// 超时端点及其测试
	http.HandleFunc("/timeout", handleTimeout)
	http.HandleFunc("/timeout/test", handleTest)

	// 延迟成功端点及其测试
	http.HandleFunc("/delayed-success", handleDelayedSuccess)
	http.HandleFunc("/delayed-success/test", handleTest)

	log.Printf("🚀 Run Task Test Platform starting on port %s", port)
	log.Printf("📋 Available endpoints:")
	log.Printf("   GET  /                    - 首页，显示使用说明")
	log.Printf("   GET  /health              - 健康检查")
	log.Printf("   POST /test                - 通用连接测试（验证 HMAC）")
	log.Printf("")
	log.Printf("   POST /success             - 立即回调成功")
	log.Printf("   POST /success/test        - 成功端点的连接测试")
	log.Printf("")
	log.Printf("   POST /failure             - 立即回调失败")
	log.Printf("   POST /failure/test        - 失败端点的连接测试")
	log.Printf("")
	log.Printf("   POST /timeout             - 不回调（模拟超时）")
	log.Printf("   POST /timeout/test        - 超时端点的连接测试")
	log.Printf("")
	log.Printf("   POST /delayed-success     - 延迟 10 秒后回调成功")
	log.Printf("   POST /delayed-success/test - 延迟端点的连接测试")
	log.Printf("")
	log.Printf("💡 配置 Run Task 时使用以下 Endpoint URL:")
	log.Printf("   成功测试: %s/success", baseURL)
	log.Printf("   失败测试: %s/failure", baseURL)
	log.Printf("   超时测试: %s/timeout", baseURL)
	log.Printf("   延迟测试: %s/delayed-success", baseURL)
	log.Printf("")
	log.Printf("🔑 HMAC Key: %s", hmacKey)
	log.Printf("")
	log.Printf("📝 保存 Run Task 时会自动调用 {endpoint_url}/test 进行验证")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// handleRoot 首页
func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <title>Run Task Test Platform</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        h1 { color: #333; }
        .endpoint { background: #f5f5f5; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .endpoint h3 { margin-top: 0; color: #0066cc; }
        code { background: #e0e0e0; padding: 2px 6px; border-radius: 3px; }
        .success { border-left: 4px solid #28a745; }
        .failure { border-left: 4px solid #dc3545; }
        .timeout { border-left: 4px solid #ffc107; }
        .delayed { border-left: 4px solid #17a2b8; }
    </style>
</head>
<body>
    <h1>🧪 Run Task Test Platform</h1>
    <p>用于测试 IaC Platform Run Task 功能的模拟第三方服务</p>
    
    <h2>可用端点</h2>
    
    <div class="endpoint success">
        <h3>POST /success</h3>
        <p>立即回调成功。适用于测试 Run Task 正常通过的场景。</p>
        <p>Endpoint URL: <code>` + baseURL + `/success</code></p>
    </div>
    
    <div class="endpoint failure">
        <h3>POST /failure</h3>
        <p>立即回调失败。适用于测试 Mandatory Run Task 阻止执行的场景。</p>
        <p>Endpoint URL: <code>` + baseURL + `/failure</code></p>
    </div>
    
    <div class="endpoint timeout">
        <h3>POST /timeout</h3>
        <p>不回调（模拟超时）。适用于测试 Run Task 超时处理的场景。</p>
        <p>Endpoint URL: <code>` + baseURL + `/timeout</code></p>
    </div>
    
    <div class="endpoint delayed">
        <h3>POST /delayed-success</h3>
        <p>延迟 10 秒后回调成功。适用于测试异步回调的场景。</p>
        <p>Endpoint URL: <code>` + baseURL + `/delayed-success</code></p>
    </div>
    
    <h2>使用方法</h2>
    <ol>
        <li>在 IaC Platform 创建 Run Task，使用上述 Endpoint URL</li>
        <li>将 Run Task 关联到 Workspace</li>
        <li>执行 Plan/Apply 任务，观察 Run Task 执行结果</li>
    </ol>
    
    <h2>健康检查</h2>
    <p>GET <code>/health</code> - 返回服务状态</p>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleHealth 健康检查
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "runtask-test-platform",
	})
}

// handleTest 连接测试端点（验证 HMAC 签名）
func handleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("❌ [TEST] Failed to read body: %v", err)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("🧪 [TEST] Received connection test request")
	log.Printf("   Body: %s", string(body))

	// 检查是否是测试请求
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("❌ [TEST] Failed to parse payload: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": "Invalid JSON payload",
		})
		return
	}

	// 验证 HMAC 签名
	signature := r.Header.Get("X-TFC-Task-Signature")
	hmacVerified := false
	hmacError := ""

	if signature != "" {
		log.Printf("   Signature header: %s", signature)
		// 解析签名（格式：sha512=xxx）
		if strings.HasPrefix(signature, "sha512=") {
			providedSig := strings.TrimPrefix(signature, "sha512=")
			expectedSig := calculateHMAC(body, hmacKey)

			if hmac.Equal([]byte(providedSig), []byte(expectedSig)) {
				hmacVerified = true
				log.Printf("   ✅ HMAC signature verified successfully")
			} else {
				hmacError = "HMAC signature mismatch"
				log.Printf("   ❌ HMAC signature mismatch")
				log.Printf("      Expected: %s", expectedSig)
				log.Printf("      Provided: %s", providedSig)
			}
		} else {
			hmacError = "Invalid signature format (expected sha512=xxx)"
			log.Printf("   ❌ Invalid signature format")
		}
	} else {
		log.Printf("     No HMAC signature provided")
		hmacError = "No signature provided"
	}

	// 检查是否是测试请求
	isTest, _ := payload["is_test"].(bool)
	stage, _ := payload["stage"].(string)

	response := map[string]interface{}{
		"status":        "acknowledged",
		"message":       "Connection test received",
		"is_test":       isTest,
		"stage":         stage,
		"hmac_verified": hmacVerified,
		"timestamp":     time.Now().Format(time.RFC3339),
	}

	if hmacError != "" {
		response["hmac_error"] = hmacError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("🧪 [TEST] Response sent: hmac_verified=%v", hmacVerified)
}

// calculateHMAC 计算 HMAC-SHA512 签名
func calculateHMAC(payload []byte, key string) string {
	h := hmac.New(sha512.New, []byte(key))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// handleSuccess 立即回调成功
func handleSuccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := parsePayload(r)
	if err != nil {
		log.Printf("❌ [SUCCESS] Failed to parse payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	log.Printf("✅ [SUCCESS] Received request for task %d, stage: %s", payload.TaskID, payload.Stage)
	log.Printf("   Callback URL: %s", payload.TaskResultCallbackURL)

	// 立即返回 200 OK
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})

	// 异步发送成功回调
	go func() {
		time.Sleep(500 * time.Millisecond) // 短暂延迟，模拟处理时间

		callback := CallbackPayload{
			Data: CallbackData{
				Type: "task-results",
				Attributes: CallbackAttributes{
					Status:  "passed",
					Message: "All checks passed successfully! ✅",
					URL:     baseURL + "/results/success",
				},
				Relationships: &CallbackRelationships{
					Outcomes: &OutcomesRelationship{
						Data: []OutcomeData{
							{
								Type: "task-result-outcomes",
								Attributes: OutcomeAttributes{
									OutcomeID:   "TEST-SUCCESS-001",
									Description: "Security scan passed",
									Body:        "# Security Scan Results\n\nAll resources passed security checks.\n\n- ✅ No public S3 buckets\n- ✅ Encryption enabled\n- ✅ IAM policies restricted",
									URL:         baseURL + "/results/security",
									Tags: map[string]interface{}{
										"Status": []map[string]interface{}{
											{"label": "Passed", "level": "info"},
										},
										"Severity": []map[string]interface{}{
											{"label": "None", "level": "none"},
										},
									},
								},
							},
							{
								Type: "task-result-outcomes",
								Attributes: OutcomeAttributes{
									OutcomeID:   "TEST-SUCCESS-002",
									Description: "Cost estimation completed",
									Body:        "# Cost Estimation\n\nEstimated monthly cost: **$45.00**\n\n| Resource | Cost |\n|----------|------|\n| EC2 | $30.00 |\n| S3 | $5.00 |\n| RDS | $10.00 |",
									URL:         baseURL + "/results/cost",
									Tags: map[string]interface{}{
										"Status": []map[string]interface{}{
											{"label": "Completed", "level": "info"},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		if err := sendCallback(payload.TaskResultCallbackURL, payload.AccessToken, callback); err != nil {
			log.Printf("❌ [SUCCESS] Failed to send callback: %v", err)
		} else {
			log.Printf("✅ [SUCCESS] Callback sent successfully for task %d", payload.TaskID)
		}
	}()
}

// handleFailure 立即回调失败
func handleFailure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := parsePayload(r)
	if err != nil {
		log.Printf("❌ [FAILURE] Failed to parse payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	log.Printf("🔴 [FAILURE] Received request for task %d, stage: %s", payload.TaskID, payload.Stage)
	log.Printf("   Callback URL: %s", payload.TaskResultCallbackURL)

	// 立即返回 200 OK
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})

	// 异步发送失败回调
	go func() {
		time.Sleep(500 * time.Millisecond) // 短暂延迟，模拟处理时间

		callback := CallbackPayload{
			Data: CallbackData{
				Type: "task-results",
				Attributes: CallbackAttributes{
					Status:  "failed",
					Message: "Security violations detected! ❌",
					URL:     baseURL + "/results/failure",
				},
				Relationships: &CallbackRelationships{
					Outcomes: &OutcomesRelationship{
						Data: []OutcomeData{
							{
								Type: "task-result-outcomes",
								Attributes: OutcomeAttributes{
									OutcomeID:   "TEST-FAIL-001",
									Description: "S3 bucket is publicly accessible",
									Body:        "# Critical Security Issue\n\n## Problem\nThe S3 bucket `aws_s3_bucket.public` is configured with public access.\n\n## Impact\n- Data exposure risk\n- Compliance violation\n\n## Recommendation\nAdd `block_public_acls = true` to the bucket configuration.",
									URL:         baseURL + "/results/s3-public",
									Tags: map[string]interface{}{
										"Status": []map[string]interface{}{
											{"label": "Failed", "level": "error"},
										},
										"Severity": []map[string]interface{}{
											{"label": "Critical", "level": "error"},
										},
									},
								},
							},
							{
								Type: "task-result-outcomes",
								Attributes: OutcomeAttributes{
									OutcomeID:   "TEST-FAIL-002",
									Description: "IAM policy too permissive",
									Body:        "# IAM Policy Issue\n\n## Problem\nThe IAM policy `aws_iam_policy.admin` grants `*:*` permissions.\n\n## Recommendation\nRestrict permissions to only required actions.",
									URL:         baseURL + "/results/iam-permissive",
									Tags: map[string]interface{}{
										"Status": []map[string]interface{}{
											{"label": "Failed", "level": "error"},
										},
										"Severity": []map[string]interface{}{
											{"label": "High", "level": "error"},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		if err := sendCallback(payload.TaskResultCallbackURL, payload.AccessToken, callback); err != nil {
			log.Printf("❌ [FAILURE] Failed to send callback: %v", err)
		} else {
			log.Printf("🔴 [FAILURE] Callback sent successfully for task %d", payload.TaskID)
		}
	}()
}

// handleTimeout 不回调（模拟超时）
func handleTimeout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := parsePayload(r)
	if err != nil {
		log.Printf("❌ [TIMEOUT] Failed to parse payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	log.Printf("⏳ [TIMEOUT] Received request for task %d, stage: %s", payload.TaskID, payload.Stage)
	log.Printf("   Callback URL: %s", payload.TaskResultCallbackURL)
	log.Printf("     Will NOT send callback (simulating timeout)")

	// 返回 200 OK，但不发送回调
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "acknowledged",
		"message": "This endpoint will NOT send a callback (timeout simulation)",
	})

	// 不发送回调，让 IaC Platform 的超时检测器处理
}

// handleDelayedSuccess 延迟回调成功
func handleDelayedSuccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	payload, err := parsePayload(r)
	if err != nil {
		log.Printf("❌ [DELAYED] Failed to parse payload: %v", err)
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	log.Printf("⏰ [DELAYED] Received request for task %d, stage: %s", payload.TaskID, payload.Stage)
	log.Printf("   Callback URL: %s", payload.TaskResultCallbackURL)
	log.Printf("   Will send callback in 10 seconds...")

	// 立即返回 200 OK
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "acknowledged",
		"message": "Will send callback in 10 seconds",
	})

	// 延迟 10 秒后发送成功回调
	go func() {
		// 先发送 running 状态
		time.Sleep(2 * time.Second)
		runningCallback := CallbackPayload{
			Data: CallbackData{
				Type: "task-results",
				Attributes: CallbackAttributes{
					Status:  "running",
					Message: "Processing... (8 seconds remaining)",
				},
			},
		}
		if err := sendCallback(payload.TaskResultCallbackURL, payload.AccessToken, runningCallback); err != nil {
			log.Printf("❌ [DELAYED] Failed to send running callback: %v", err)
		} else {
			log.Printf("⏰ [DELAYED] Running callback sent for task %d", payload.TaskID)
		}

		// 再等待 8 秒
		time.Sleep(8 * time.Second)

		// 发送成功回调
		callback := CallbackPayload{
			Data: CallbackData{
				Type: "task-results",
				Attributes: CallbackAttributes{
					Status:  "passed",
					Message: "Delayed check completed successfully! ✅",
					URL:     baseURL + "/results/delayed",
				},
				Relationships: &CallbackRelationships{
					Outcomes: &OutcomesRelationship{
						Data: []OutcomeData{
							{
								Type: "task-result-outcomes",
								Attributes: OutcomeAttributes{
									OutcomeID:   "TEST-DELAYED-001",
									Description: "Async processing completed",
									Body:        "# Async Processing Results\n\nThe delayed check has completed successfully after 10 seconds of processing.",
									Tags: map[string]interface{}{
										"Status": []map[string]interface{}{
											{"label": "Passed", "level": "info"},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		if err := sendCallback(payload.TaskResultCallbackURL, payload.AccessToken, callback); err != nil {
			log.Printf("❌ [DELAYED] Failed to send final callback: %v", err)
		} else {
			log.Printf("⏰ [DELAYED] Final callback sent successfully for task %d", payload.TaskID)
		}
	}()
}

// parsePayload 解析请求体
func parsePayload(r *http.Request) (*RunTaskPayload, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	defer r.Body.Close()

	log.Printf("📥 Received payload: %s", string(body))

	var payload RunTaskPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	return &payload, nil
}

// sendCallback 发送回调请求
func sendCallback(callbackURL, accessToken string, payload CallbackPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal callback: %w", err)
	}

	log.Printf("📤 Sending callback to: %s", callbackURL)
	log.Printf("   Payload: %s", string(body))

	// 使用 POST 方法，因为某些环境下 PATCH 方法可能有问题
	req, err := http.NewRequest(http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("📥 Callback response: %d - %s", resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("callback returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// getEnv 获取环境变量，带默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
