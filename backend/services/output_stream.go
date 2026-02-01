package services

import (
	"log"
	"sync"
	"time"
)

// OutputMessage 输出消息
type OutputMessage struct {
	Type      string    `json:"type"`               // output, error, completed, stage_marker
	Line      string    `json:"line"`               // 输出行内容
	Timestamp time.Time `json:"timestamp"`          // 时间戳
	LineNum   int       `json:"line_num,omitempty"` // 行号
	Stage     string    `json:"stage,omitempty"`    // 阶段名称（仅stage_marker类型）
	Status    string    `json:"status,omitempty"`   // begin或end（仅stage_marker类型）
}

// Client WebSocket客户端
type Client struct {
	ID          string
	Channel     chan OutputMessage
	ConnectedAt time.Time
}

// RingBuffer 环形缓冲区（保存最近N行）
type RingBuffer struct {
	lines    []OutputMessage
	capacity int
	head     int
	size     int
	mutex    sync.RWMutex
}

// NewRingBuffer 创建环形缓冲区
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		lines:    make([]OutputMessage, capacity),
		capacity: capacity,
		head:     0,
		size:     0,
	}
}

// Add 添加消息
func (rb *RingBuffer) Add(msg OutputMessage) {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	rb.lines[rb.head] = msg
	rb.head = (rb.head + 1) % rb.capacity

	if rb.size < rb.capacity {
		rb.size++
	}
}

// GetAll 获取所有消息
func (rb *RingBuffer) GetAll() []OutputMessage {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()

	if rb.size == 0 {
		return []OutputMessage{}
	}

	result := make([]OutputMessage, rb.size)

	if rb.size < rb.capacity {
		// 未满，从0到size
		copy(result, rb.lines[:rb.size])
	} else {
		// 已满，从head开始读取
		copy(result, rb.lines[rb.head:])
		copy(result[rb.capacity-rb.head:], rb.lines[:rb.head])
	}

	return result
}

// Size 获取当前大小
func (rb *RingBuffer) Size() int {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()
	return rb.size
}

// OutputStream 单个任务的输出流
type OutputStream struct {
	TaskID    uint
	Clients   map[string]*Client
	Buffer    *RingBuffer
	mutex     sync.RWMutex
	closed    bool
	startTime time.Time
}

// NewOutputStream 创建输出流
func NewOutputStream(taskID uint) *OutputStream {
	return &OutputStream{
		TaskID:    taskID,
		Clients:   make(map[string]*Client),
		Buffer:    NewRingBuffer(1000), // 保存最近1000行
		startTime: time.Now(),
	}
}

// Subscribe 订阅输出流
func (s *OutputStream) Subscribe(clientID string) (*Client, []OutputMessage) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return nil, nil
	}

	// 创建客户端
	client := &Client{
		ID:          clientID,
		Channel:     make(chan OutputMessage, 100),
		ConnectedAt: time.Now(),
	}

	s.Clients[clientID] = client

	// 返回历史消息（最近1000行）
	history := s.Buffer.GetAll()

	log.Printf("Client %s subscribed to task %d, sent %d history lines",
		clientID, s.TaskID, len(history))

	return client, history
}

// Unsubscribe 取消订阅
func (s *OutputStream) Unsubscribe(clientID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if client, ok := s.Clients[clientID]; ok {
		close(client.Channel)
		delete(s.Clients, clientID)
		log.Printf("Client %s unsubscribed from task %d", clientID, s.TaskID)
	}
}

// Broadcast 广播消息到所有客户端
func (s *OutputStream) Broadcast(msg OutputMessage) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// 保存到缓冲区
	s.Buffer.Add(msg)

	// 广播到所有客户端
	for clientID, client := range s.Clients {
		select {
		case client.Channel <- msg:
			// 发送成功
		default:
			// 通道满了，记录警告但不阻塞
			log.Printf("Warning: Client %s channel full, dropping message", clientID)
		}
	}
}

// Close 关闭输出流
func (s *OutputStream) Close() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return
	}

	s.closed = true

	// 关闭所有客户端通道
	for _, client := range s.Clients {
		close(client.Channel)
	}

	s.Clients = make(map[string]*Client)

	log.Printf("OutputStream for task %d closed", s.TaskID)
}

// GetStats 获取统计信息
func (s *OutputStream) GetStats() map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return map[string]interface{}{
		"task_id":       s.TaskID,
		"clients_count": len(s.Clients),
		"buffer_size":   s.Buffer.Size(),
		"uptime":        time.Since(s.startTime).Seconds(),
		"closed":        s.closed,
	}
}

// GetBufferedLogs 获取缓冲区中的所有日志（用于取消任务时保存）
func (s *OutputStream) GetBufferedLogs() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	messages := s.Buffer.GetAll()
	if len(messages) == 0 {
		return ""
	}

	var result string
	for _, msg := range messages {
		result += msg.Line + "\n"
	}

	return result
}

// OutputStreamManager 管理所有任务的输出流
type OutputStreamManager struct {
	streams map[uint]*OutputStream
	mutex   sync.RWMutex
}

// NewOutputStreamManager 创建流管理器
func NewOutputStreamManager() *OutputStreamManager {
	return &OutputStreamManager{
		streams: make(map[uint]*OutputStream),
	}
}

// GetOrCreate 获取或创建输出流
func (m *OutputStreamManager) GetOrCreate(taskID uint) *OutputStream {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if stream, ok := m.streams[taskID]; ok {
		return stream
	}

	stream := NewOutputStream(taskID)
	m.streams[taskID] = stream

	log.Printf("Created OutputStream for task %d", taskID)

	return stream
}

// Get 获取输出流
func (m *OutputStreamManager) Get(taskID uint) *OutputStream {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.streams[taskID]
}

// Close 关闭输出流
func (m *OutputStreamManager) Close(taskID uint) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if stream, ok := m.streams[taskID]; ok {
		stream.Close()
		delete(m.streams, taskID)
		log.Printf("Closed OutputStream for task %d", taskID)
	}
}

// GetAllStats 获取所有流的统计信息
func (m *OutputStreamManager) GetAllStats() []map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := make([]map[string]interface{}, 0, len(m.streams))
	for _, stream := range m.streams {
		stats = append(stats, stream.GetStats())
	}

	return stats
}

// Cleanup 清理超时的流（定期调用）
func (m *OutputStreamManager) Cleanup(timeout time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	for taskID, stream := range m.streams {
		if stream.closed && now.Sub(stream.startTime) > timeout {
			delete(m.streams, taskID)
			log.Printf("Cleaned up OutputStream for task %d", taskID)
		}
	}
}

// StartCleanupWorker 启动清理worker
func (m *OutputStreamManager) StartCleanupWorker() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			m.Cleanup(30 * time.Minute)
		}
	}()
}
