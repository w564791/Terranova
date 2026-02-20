package services

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// AIMetrics AI 服务的 Prometheus 指标
// 简单实现，不依赖 prometheus 客户端库
type AIMetrics struct {
	mu sync.RWMutex

	// Histogram 数据：记录耗时分布
	// key: metric_name + labels
	histograms map[string]*HistogramData

	// Counter 数据：记录调用次数
	counters map[string]float64

	// Gauge 数据：记录当前值
	gauges map[string]float64
}

// HistogramData 直方图数据
type HistogramData struct {
	Count   uint64
	Sum     float64
	Buckets map[float64]uint64 // bucket 上限 -> 计数
}

// 默认的 bucket 边界（毫秒）
var defaultBuckets = []float64{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

// 全局 AI 指标实例
var aiMetrics *AIMetrics
var aiMetricsOnce sync.Once

// GetAIMetrics 获取全局 AI 指标实例
func GetAIMetrics() *AIMetrics {
	aiMetricsOnce.Do(func() {
		aiMetrics = &AIMetrics{
			histograms: make(map[string]*HistogramData),
			counters:   make(map[string]float64),
			gauges:     make(map[string]float64),
		}
	})
	return aiMetrics
}

// RecordDuration 记录耗时（毫秒）
func (m *AIMetrics) RecordDuration(name string, labels map[string]string, durationMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, labels)
	hist, ok := m.histograms[key]
	if !ok {
		hist = &HistogramData{
			Buckets: make(map[float64]uint64),
		}
		for _, b := range defaultBuckets {
			hist.Buckets[b] = 0
		}
		m.histograms[key] = hist
	}

	hist.Count++
	hist.Sum += durationMs

	// 更新 bucket 计数
	for _, b := range defaultBuckets {
		if durationMs <= b {
			hist.Buckets[b]++
		}
	}
}

// IncCounter 增加计数器
func (m *AIMetrics) IncCounter(name string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, labels)
	m.counters[key]++
}

// AddCounter 增加计数器指定值
func (m *AIMetrics) AddCounter(name string, labels map[string]string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, labels)
	m.counters[key] += value
}

// SetGauge 设置 Gauge 值
func (m *AIMetrics) SetGauge(name string, labels map[string]string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(name, labels)
	m.gauges[key] = value
}

// buildKey 构建指标 key
func (m *AIMetrics) buildKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}

	// 按 key 排序，确保一致性
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(labels))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}

	return fmt.Sprintf("%s{%s}", name, strings.Join(parts, ","))
}

// Export 导出 Prometheus 格式的指标
func (m *AIMetrics) Export() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sb strings.Builder

	// 导出 Histogram
	for key, hist := range m.histograms {
		name, labels := m.parseKey(key)

		// 写入 HELP 和 TYPE（简化版，只写一次）
		sb.WriteString(fmt.Sprintf("# HELP %s AI service duration in milliseconds\n", name))
		sb.WriteString(fmt.Sprintf("# TYPE %s histogram\n", name))

		// 写入 bucket
		bucketKeys := make([]float64, 0, len(hist.Buckets))
		for b := range hist.Buckets {
			bucketKeys = append(bucketKeys, b)
		}
		sort.Float64s(bucketKeys)

		cumulative := uint64(0)
		for _, b := range bucketKeys {
			cumulative += hist.Buckets[b]
			bucketLabels := m.addLabel(labels, "le", fmt.Sprintf("%.0f", b))
			sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", name, bucketLabels, cumulative))
		}

		// +Inf bucket
		infLabels := m.addLabel(labels, "le", "+Inf")
		sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", name, infLabels, hist.Count))

		// sum 和 count
		sb.WriteString(fmt.Sprintf("%s_sum%s %.2f\n", name, labels, hist.Sum))
		sb.WriteString(fmt.Sprintf("%s_count%s %d\n", name, labels, hist.Count))
	}

	// 导出 Counter
	for key, value := range m.counters {
		name, labels := m.parseKey(key)
		sb.WriteString(fmt.Sprintf("# HELP %s AI service counter\n", name))
		sb.WriteString(fmt.Sprintf("# TYPE %s counter\n", name))
		sb.WriteString(fmt.Sprintf("%s%s %.0f\n", name, labels, value))
	}

	// 导出 Gauge
	for key, value := range m.gauges {
		name, labels := m.parseKey(key)
		sb.WriteString(fmt.Sprintf("# HELP %s AI service gauge\n", name))
		sb.WriteString(fmt.Sprintf("# TYPE %s gauge\n", name))
		sb.WriteString(fmt.Sprintf("%s%s %.2f\n", name, labels, value))
	}

	return sb.String()
}

// parseKey 解析 key 为 name 和 labels
func (m *AIMetrics) parseKey(key string) (string, string) {
	idx := strings.Index(key, "{")
	if idx == -1 {
		return key, ""
	}
	return key[:idx], key[idx:]
}

// addLabel 添加 label 到现有 labels
func (m *AIMetrics) addLabel(labels string, key, value string) string {
	newLabel := fmt.Sprintf("%s=%q", key, value)
	if labels == "" {
		return "{" + newLabel + "}"
	}
	// 移除最后的 }，添加新 label
	return labels[:len(labels)-1] + "," + newLabel + "}"
}

// MetricsHandler 返回 Prometheus 指标的 HTTP Handler
func MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(GetAIMetrics().Export()))
	}
}

// ========== 便捷方法 ==========

// RecordAICallDuration 记录 AI 调用耗时
func RecordAICallDuration(capability, stage string, durationMs float64) {
	GetAIMetrics().RecordDuration("iac_ai_call_duration_ms", map[string]string{
		"capability": capability,
		"stage":      stage,
	}, durationMs)
}

// RecordVectorSearchDuration 记录向量搜索耗时
func RecordVectorSearchDuration(resourceType, stage string, durationMs float64) {
	GetAIMetrics().RecordDuration("iac_vector_search_duration_ms", map[string]string{
		"resource_type": resourceType,
		"stage":         stage,
	}, durationMs)
}

// RecordSkillAssemblyDuration 记录 Skill 组装耗时
func RecordSkillAssemblyDuration(capability string, skillCount int, durationMs float64) {
	GetAIMetrics().RecordDuration("iac_skill_assembly_duration_ms", map[string]string{
		"capability":  capability,
		"skill_count": fmt.Sprintf("%d", skillCount),
	}, durationMs)
}

// IncAICallCount 增加 AI 调用计数
func IncAICallCount(capability, status string) {
	GetAIMetrics().IncCounter("iac_ai_call_total", map[string]string{
		"capability": capability,
		"status":     status,
	})
}

// IncVectorSearchCount 增加向量搜索计数
func IncVectorSearchCount(resourceType string, found bool) {
	status := "not_found"
	if found {
		status = "found"
	}
	GetAIMetrics().IncCounter("iac_vector_search_total", map[string]string{
		"resource_type": resourceType,
		"status":        status,
	})
}

// RecordParallelExecutionDuration 记录并行执行耗时
func RecordParallelExecutionDuration(task, status string, durationMs float64) {
	GetAIMetrics().RecordDuration("iac_parallel_execution_ms", map[string]string{
		"task":   task,
		"status": status,
	}, durationMs)
}

// RecordDomainSkillSelection 记录 Domain Skill 选择
func RecordDomainSkillSelection(skillCount int, method string, durationMs float64) {
	GetAIMetrics().RecordDuration("iac_domain_skill_selection_ms", map[string]string{
		"skill_count": fmt.Sprintf("%d", skillCount),
		"method":      method,
	}, durationMs)
}

// RecordCMDBAssessment 记录 CMDB 评估
func RecordCMDBAssessment(needCMDB bool, resourceTypeCount int, method string, durationMs float64) {
	GetAIMetrics().RecordDuration("iac_cmdb_assessment_ms", map[string]string{
		"need_cmdb":           fmt.Sprintf("%t", needCMDB),
		"resource_type_count": fmt.Sprintf("%d", resourceTypeCount),
		"method":              method,
	}, durationMs)
}

// IncCMDBQueryCount 增加 CMDB 查询计数
func IncCMDBQueryCount(resourceType string, found bool, candidateCount int) {
	status := "not_found"
	if found {
		if candidateCount > 1 {
			status = "multiple"
		} else {
			status = "found"
		}
	}
	GetAIMetrics().IncCounter("iac_cmdb_query_total", map[string]string{
		"resource_type":   resourceType,
		"status":          status,
		"candidate_count": fmt.Sprintf("%d", candidateCount),
	})
}

// SetActiveParallelTasks 设置当前活跃的并行任务数
func SetActiveParallelTasks(count int) {
	GetAIMetrics().SetGauge("iac_active_parallel_tasks", map[string]string{}, float64(count))
}

// Timer 计时器，用于方便地记录耗时
type Timer struct {
	start time.Time
}

// NewTimer 创建新的计时器
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// ElapsedMs 返回经过的毫秒数
func (t *Timer) ElapsedMs() float64 {
	return float64(time.Since(t.start).Milliseconds())
}
