package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"iac-platform/internal/models"

	"github.com/fsnotify/fsnotify"
)

// StateFileWatcher 监控 terraform.tfstate 文件变更，实时 push temp state 到数据库
type StateFileWatcher struct {
	workDir      string
	workspaceID  string
	taskID       uint
	createdBy    *string
	dataAccessor DataAccessor

	watcher *fsnotify.Watcher
	stopCh  chan struct{}
	done    chan struct{}

	mu           sync.Mutex
	tempRecordID uint
	lastChecksum string

	stopOnce sync.Once
}

// NewStateFileWatcher 创建 watcher 实例
func NewStateFileWatcher(
	workDir, workspaceID string,
	taskID uint,
	createdBy *string,
	dataAccessor DataAccessor,
) *StateFileWatcher {
	return &StateFileWatcher{
		workDir:      workDir,
		workspaceID:  workspaceID,
		taskID:       taskID,
		createdBy:    createdBy,
		dataAccessor: dataAccessor,
		stopCh:       make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Start 启动文件监控（非阻塞）
func (w *StateFileWatcher) Start() error {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// 监听工作目录（不是单个文件，因为 terraform 使用原子 rename）
	if err := fsWatcher.Add(w.workDir); err != nil {
		fsWatcher.Close()
		return err
	}

	w.watcher = fsWatcher
	go w.watchLoop()

	log.Printf("[StateWatcher] Started watching %s for workspace %s (task %d)",
		w.workDir, w.workspaceID, w.taskID)
	return nil
}

// Stop 停止监控，等待 goroutine 完成 drain + 最终 push 后退出
func (w *StateFileWatcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		<-w.done // 阻塞直到 watchLoop 退出（含 drainAndFinalPush）
		w.watcher.Close()
		log.Printf("[StateWatcher] Stopped for workspace %s (task %d)", w.workspaceID, w.taskID)
	})
}

// Promote 将 temp 记录提升为正式版本
func (w *StateFileWatcher) Promote() error {
	w.mu.Lock()
	recordID := w.tempRecordID
	w.mu.Unlock()

	if recordID == 0 {
		log.Printf("[StateWatcher] No temp record to promote for workspace %s", w.workspaceID)
		return nil
	}

	if err := w.dataAccessor.PromoteTempState(w.workspaceID, recordID); err != nil {
		return err
	}

	log.Printf("[StateWatcher] Promoted temp state #%d for workspace %s", recordID, w.workspaceID)
	return nil
}

// GetTempRecordID 返回当前 temp 记录的数据库 ID（0 表示未创建）
func (w *StateFileWatcher) GetTempRecordID() uint {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.tempRecordID
}

// watchLoop 事件循环
func (w *StateFileWatcher) watchLoop() {
	defer close(w.done)

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) != "terraform.tfstate" {
				continue
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Rename) {
				w.pushTempState()
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[StateWatcher] fsnotify error: %v", err)

		case <-w.stopCh:
			w.drainAndFinalPush()
			return
		}
	}
}

// drainAndFinalPush drain 残留事件并做最终一次 push
func (w *StateFileWatcher) drainAndFinalPush() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				goto finalPush
			}
			if filepath.Base(event.Name) == "terraform.tfstate" {
				if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Rename) {
					w.pushTempState()
				}
			}
		default:
			goto finalPush
		}
	}

finalPush:
	w.pushTempState()
	log.Printf("[StateWatcher] Final push completed for workspace %s", w.workspaceID)
}

// pushTempState 读取 state 文件并 upsert 到数据库
func (w *StateFileWatcher) pushTempState() {
	stateFile := filepath.Join(w.workDir, "terraform.tfstate")

	stateData, err := os.ReadFile(stateFile)
	if err != nil {
		return
	}

	if len(stateData) == 0 {
		return
	}

	// checksum 去重
	checksum := stateFileChecksum(stateData)
	w.mu.Lock()
	if checksum == w.lastChecksum {
		w.mu.Unlock()
		return
	}
	currentTempID := w.tempRecordID
	w.mu.Unlock()

	var stateContent models.JSONB
	if err := json.Unmarshal(stateData, &stateContent); err != nil {
		log.Printf("[StateWatcher] Failed to parse state JSON: %v", err)
		return
	}

	// 从 state JSON 中提取元数据
	var lineage string
	var serial int
	var resourceCount int
	if l, ok := stateContent["lineage"].(string); ok {
		lineage = l
	}
	if s, ok := stateContent["serial"].(float64); ok {
		serial = int(s)
	}
	if r, ok := stateContent["resources"].([]interface{}); ok {
		resourceCount = len(r)
	}

	record := &models.WorkspaceStateVersion{
		WorkspaceID:   w.workspaceID,
		Content:       stateContent,
		Checksum:      checksum,
		SizeBytes:     len(stateData),
		TaskID:        &w.taskID,
		CreatedBy:     w.createdBy,
		IsTemp:        true,
		Lineage:       lineage,
		Serial:        serial,
		ResourceCount: resourceCount,
	}

	if currentTempID == 0 {
		maxVersion, err := w.dataAccessor.GetMaxStateVersion(w.workspaceID)
		if err != nil {
			log.Printf("[StateWatcher] Failed to get max version: %v", err)
			return
		}
		record.Version = maxVersion + 1
	} else {
		record.ID = currentTempID
	}

	if err := w.dataAccessor.UpsertTempState(record); err != nil {
		log.Printf("[StateWatcher] Failed to upsert temp state: %v", err)
		return
	}

	w.mu.Lock()
	if w.tempRecordID == 0 {
		w.tempRecordID = record.ID
		log.Printf("[StateWatcher] Created temp state version #%d for workspace %s",
			record.Version, w.workspaceID)
	} else {
		log.Printf("[StateWatcher] Updated temp state for workspace %s (%.1f KB)",
			w.workspaceID, float64(len(stateData))/1024)
	}
	w.lastChecksum = checksum
	w.mu.Unlock()
}

// stateFileChecksum 计算 state 文件的 SHA256 校验和
func stateFileChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
