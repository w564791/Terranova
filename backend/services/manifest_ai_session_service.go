package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ManifestAISessionService manifest AI 会话/消息持久化(按 manifest+用户隔离)。
type ManifestAISessionService struct {
	db *gorm.DB
}

func NewManifestAISessionService(db *gorm.DB) *ManifestAISessionService {
	return &ManifestAISessionService{db: db}
}

func newSessionID() string { return "mas-" + uuid.New().String() }
func newMessageID() string { return "mam-" + uuid.New().String() }

// ListSessions 列出某用户在某 manifest 下的会话(按 updated_at 倒序)。
func (s *ManifestAISessionService) ListSessions(manifestID, userID string) ([]models.ManifestAISession, error) {
	var out []models.ManifestAISession
	err := s.db.Where("manifest_id = ? AND user_id = ?", manifestID, userID).
		Order("updated_at DESC").Find(&out).Error
	return out, err
}

// CreateSession 新建会话。
func (s *ManifestAISessionService) CreateSession(manifestID, orgID, userID, title string) (*models.ManifestAISession, error) {
	sess := &models.ManifestAISession{
		ID:         newSessionID(),
		ManifestID: manifestID,
		OrgID:      orgID,
		UserID:     userID,
		Title:      title,
	}
	if err := s.db.Create(sess).Error; err != nil {
		return nil, err
	}
	return sess, nil
}

// GetMessages 拉某会话消息(校验 owner=userID,防越权)。
func (s *ManifestAISessionService) GetMessages(sessionID, userID string) ([]models.ManifestAIMessage, error) {
	if _, err := s.ownedSession(sessionID, userID); err != nil {
		return nil, err
	}
	var out []models.ManifestAIMessage
	err := s.db.Where("session_id = ?", sessionID).Order("created_at ASC").Find(&out).Error
	return out, err
}

// DeleteSession 删会话及其消息(owner 校验)。
func (s *ManifestAISessionService) DeleteSession(sessionID, userID string) error {
	if _, err := s.ownedSession(sessionID, userID); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("session_id = ?", sessionID).Delete(&models.ManifestAIMessage{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", sessionID).Delete(&models.ManifestAISession{}).Error
	})
}

// AppendExchange 往会话追加一次「用户消息 + AI 产出」,并更新 session.updated_at。
// 仅当 sessionID 非空且属于该用户时落库;否则静默跳过(向后兼容:不带 session 不持久化)。
// userContent / assistantContent 为已序列化的 JSON 字符串。
func (s *ManifestAISessionService) AppendExchange(sessionID, userID, kind, userContent, assistantContent string) {
	if sessionID == "" {
		return
	}
	sess, err := s.ownedSession(sessionID, userID)
	if err != nil {
		return // 会话不存在/不属于该用户,跳过(不阻断主流程)
	}
	msgs := []models.ManifestAIMessage{
		{ID: newMessageID(), SessionID: sessionID, Role: "user", Kind: kind, Content: userContent},
		{ID: newMessageID(), SessionID: sessionID, Role: "assistant", Kind: kind, Content: assistantContent},
	}
	if err := s.db.Create(&msgs).Error; err != nil {
		return
	}
	// touch updated_at(让最近活跃的会话排前)
	s.db.Model(&models.ManifestAISession{}).Where("id = ?", sess.ID).Update("updated_at", gorm.Expr("now()"))
}

// ownedSession 取会话并校验归属;不属于该用户视为不存在。
func (s *ManifestAISessionService) ownedSession(sessionID, userID string) (*models.ManifestAISession, error) {
	var sess models.ManifestAISession
	if err := s.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&sess).Error; err != nil {
		return nil, fmt.Errorf("会话不存在或无权访问")
	}
	return &sess, nil
}

// MarshalJSONContent 把任意结构序列化为存储用的 JSON 字符串(失败返回 "{}")。
func MarshalJSONContent(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
