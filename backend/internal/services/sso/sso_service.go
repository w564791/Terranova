package sso

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// SSOService SSO 核心服务
type SSOService struct {
	db      *gorm.DB
	factory *DefaultProviderFactory
}

// StateData state 参数关联的数据（用于内部传递，实际存储在 PostgreSQL sso_states 表）
type StateData struct {
	ProviderKey string    `json:"provider_key"`
	RedirectURL string    `json:"redirect_url"` // 前端回调后跳转的 URL
	Action      string    `json:"action"`       // "login" 或 "link"
	UserID      string    `json:"user_id"`      // link 操作时的用户 ID
	CreatedAt   time.Time `json:"created_at"`
}

// LoginResult SSO 登录结果
type LoginResult struct {
	User      *models.User `json:"user"`
	IsNewUser bool         `json:"is_new_user"`
	IsLinked  bool         `json:"is_linked"` // 是否为已有用户关联新身份
}

// NewSSOService 创建 SSO 服务
func NewSSOService(db *gorm.DB) *SSOService {
	svc := &SSOService{
		db:      db,
		factory: NewProviderFactory(),
	}

	// 启动过期 state 清理协程（从数据库清理）
	go svc.cleanupExpiredStates()

	return svc
}

// ============================================
// Provider 配置管理
// ============================================

// GetEnabledProviders 获取所有启用的 Provider（用于登录页展示）
func (s *SSOService) GetEnabledProviders() ([]models.SSOProviderPublic, error) {
	var providers []models.SSOProvider
	err := s.db.Where("is_enabled = ? AND show_on_login_page = ?", true, true).
		Order("display_order ASC").
		Find(&providers).Error
	if err != nil {
		return nil, err
	}

	result := make([]models.SSOProviderPublic, len(providers))
	for i, p := range providers {
		result[i] = p.ToPublic()
	}
	return result, nil
}

// GetProviderConfig 获取 Provider 配置
func (s *SSOService) GetProviderConfig(providerKey string) (*models.SSOProvider, error) {
	var provider models.SSOProvider
	err := s.db.Where("provider_key = ? AND is_enabled = ?", providerKey, true).First(&provider).Error
	if err != nil {
		return nil, fmt.Errorf("provider %s not found or disabled", providerKey)
	}
	return &provider, nil
}

// GetAllProviders 获取所有 Provider 配置（管理员用）
func (s *SSOService) GetAllProviders() ([]models.SSOProvider, error) {
	var providers []models.SSOProvider
	err := s.db.Order("display_order ASC").Find(&providers).Error
	return providers, err
}

// CreateProvider 创建 Provider 配置
func (s *SSOService) CreateProvider(provider *models.SSOProvider) error {
	if err := ValidateProviderConfig(provider); err != nil {
		return err
	}
	return s.db.Create(provider).Error
}

// UpdateProvider 更新 Provider 配置
func (s *SSOService) UpdateProvider(id int64, updates map[string]interface{}) error {
	return s.db.Model(&models.SSOProvider{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteProvider 删除 Provider 配置
func (s *SSOService) DeleteProvider(id int64) error {
	return s.db.Delete(&models.SSOProvider{}, id).Error
}

// ============================================
// SSO 登录流程
// ============================================

// GenerateAuthURL 生成授权 URL
func (s *SSOService) GenerateAuthURL(providerKey string, frontendRedirectURL string, action string, userID string) (string, string, error) {
	providerCfg, err := s.GetProviderConfig(providerKey)
	if err != nil {
		return "", "", err
	}

	provider, err := s.factory.CreateProvider(providerCfg)
	if err != nil {
		return "", "", fmt.Errorf("failed to create provider: %w", err)
	}

	// 生成 state
	state, err := generateState()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate state: %w", err)
	}

	// 存储 state
	s.storeState(state, &StateData{
		ProviderKey: providerKey,
		RedirectURL: frontendRedirectURL,
		Action:      action,
		UserID:      userID,
		CreatedAt:   time.Now(),
	})

	// 生成授权 URL
	authURL, err := provider.GetAuthorizationURL(state, providerCfg.CallbackURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate auth URL: %w", err)
	}

	return authURL, state, nil
}

// HandleCallback 处理 SSO 回调
func (s *SSOService) HandleCallback(ctx context.Context, providerKey string, code string, state string, ipAddress string, userAgent string) (*LoginResult, error) {
	// 1. 验证 state
	stateData, err := s.validateState(state)
	if err != nil {
		s.logLogin(providerKey, "", "", "", "failed", "invalid state: "+err.Error(), ipAddress, userAgent)
		return nil, fmt.Errorf("invalid state: %w", err)
	}

	// 验证 provider_key 一致性
	if stateData.ProviderKey != providerKey {
		s.logLogin(providerKey, "", "", "", "failed", "provider key mismatch", ipAddress, userAgent)
		return nil, fmt.Errorf("provider key mismatch")
	}

	// 2. 获取 Provider 配置
	providerCfg, err := s.GetProviderConfig(providerKey)
	if err != nil {
		s.logLogin(providerKey, "", "", "", "failed", "provider not found: "+err.Error(), ipAddress, userAgent)
		return nil, err
	}

	// 3. 创建 Provider 实例
	provider, err := s.factory.CreateProvider(providerCfg)
	if err != nil {
		s.logLogin(providerKey, "", "", "", "failed", "create provider failed: "+err.Error(), ipAddress, userAgent)
		return nil, err
	}

	// 4. 用 code 换取 token
	token, err := provider.ExchangeCode(ctx, code, providerCfg.CallbackURL)
	if err != nil {
		s.logLogin(providerKey, "", "", "", "failed", "exchange code failed: "+err.Error(), ipAddress, userAgent)
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// 5. 获取用户信息
	userInfo, err := provider.GetUserInfo(ctx, token)
	if err != nil {
		s.logLogin(providerKey, "", "", "", "failed", "get user info failed: "+err.Error(), ipAddress, userAgent)
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// 6. 验证邮箱域名（企业 SSO）
	if len(providerCfg.AllowedDomains) > 0 {
		if !isEmailDomainAllowed(userInfo.Email, providerCfg.AllowedDomains) {
			s.logLogin(providerKey, "", userInfo.ProviderUserID, userInfo.Email, "failed", "email domain not allowed", ipAddress, userAgent)
			return nil, fmt.Errorf("email domain not allowed")
		}
	}

	// 7. 根据 action 处理
	if stateData.Action == "link" {
		// 绑定操作
		return s.handleLinkCallback(ctx, stateData.UserID, providerKey, providerCfg, userInfo, token, ipAddress, userAgent)
	}

	// 登录操作
	return s.handleLoginCallback(ctx, providerKey, providerCfg, userInfo, token, ipAddress, userAgent)
}

// handleLoginCallback 处理登录回调
func (s *SSOService) handleLoginCallback(ctx context.Context, providerKey string, providerCfg *models.SSOProvider, userInfo *StandardUserInfo, token *OAuthToken, ipAddress string, userAgent string) (*LoginResult, error) {
	// 查找已有身份关联
	var identity models.UserIdentity
	err := s.db.Where("provider = ? AND provider_user_id = ?", providerKey, userInfo.ProviderUserID).First(&identity).Error

	if err == nil {
		// 找到已有关联 -> 直接登录
		var user models.User
		if err := s.db.Where("user_id = ? AND is_active = ?", identity.UserID, true).First(&user).Error; err != nil {
			s.logLogin(providerKey, identity.UserID, userInfo.ProviderUserID, userInfo.Email, "failed", "user not found or inactive", ipAddress, userAgent)
			return nil, fmt.Errorf("user not found or inactive")
		}

		// 更新身份信息
		now := time.Now()
		updates := map[string]interface{}{
			"last_used_at":    now,
			"provider_email":  userInfo.Email,
			"provider_name":   userInfo.Name,
			"provider_avatar": userInfo.Avatar,
		}
		if rawJSON, err := json.Marshal(userInfo.RawData); err == nil {
			updates["raw_data"] = rawJSON
		}
		s.db.Model(&identity).Updates(updates)

		// 保存加密 token
		s.saveTokens(&identity, token)

		// 更新用户最后登录时间
		s.db.Model(&user).Update("last_login_at", now)

		s.logLogin(providerKey, user.ID, userInfo.ProviderUserID, userInfo.Email, "success", "", ipAddress, userAgent)

		return &LoginResult{User: &user, IsNewUser: false}, nil
	}

	// 未找到关联 -> 尝试通过邮箱匹配用户（仅当邮箱已验证时才自动关联）
	if userInfo.Email != "" && userInfo.EmailVerified {
		var existingUser models.User
		if err := s.db.Where("email = ? AND is_active = ?", userInfo.Email, true).First(&existingUser).Error; err == nil {
			// 找到同邮箱用户 -> 创建身份关联
			newIdentity, err := s.createIdentity(existingUser.ID, providerKey, userInfo, token, false)
			if err != nil {
				s.logLogin(providerKey, existingUser.ID, userInfo.ProviderUserID, userInfo.Email, "failed", "create identity failed: "+err.Error(), ipAddress, userAgent)
				return nil, err
			}

			// 更新 users 表的 OAuth 字段（如果为空）
			if existingUser.OAuthProvider == "" {
				s.db.Model(&existingUser).Updates(map[string]interface{}{
					"oauth_provider": providerKey,
					"oauth_id":       userInfo.ProviderUserID,
				})
			}

			now := time.Now()
			s.db.Model(&existingUser).Update("last_login_at", now)

			s.logLogin(providerKey, existingUser.ID, userInfo.ProviderUserID, userInfo.Email, "user_linked", "", ipAddress, userAgent)
			_ = newIdentity

			return &LoginResult{User: &existingUser, IsNewUser: false, IsLinked: true}, nil
		}
	}

	// 未找到用户 -> 检查是否允许自动创建
	if !providerCfg.AutoCreateUser {
		s.logLogin(providerKey, "", userInfo.ProviderUserID, userInfo.Email, "failed", "auto create user disabled", ipAddress, userAgent)
		return nil, fmt.Errorf("user not registered, please contact administrator")
	}

	// 自动创建用户
	user, err := s.createSSOUser(providerKey, providerCfg, userInfo)
	if err != nil {
		s.logLogin(providerKey, "", userInfo.ProviderUserID, userInfo.Email, "failed", "create user failed: "+err.Error(), ipAddress, userAgent)
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// 创建身份关联
	_, err = s.createIdentity(user.ID, providerKey, userInfo, token, true)
	if err != nil {
		s.logLogin(providerKey, user.ID, userInfo.ProviderUserID, userInfo.Email, "failed", "create identity failed: "+err.Error(), ipAddress, userAgent)
		return nil, err
	}

	s.logLogin(providerKey, user.ID, userInfo.ProviderUserID, userInfo.Email, "user_created", "", ipAddress, userAgent)

	return &LoginResult{User: user, IsNewUser: true}, nil
}

// handleLinkCallback 处理绑定回调
func (s *SSOService) handleLinkCallback(ctx context.Context, userID string, providerKey string, providerCfg *models.SSOProvider, userInfo *StandardUserInfo, token *OAuthToken, ipAddress string, userAgent string) (*LoginResult, error) {
	// 检查该身份是否已被其他用户绑定
	var existingIdentity models.UserIdentity
	err := s.db.Where("provider = ? AND provider_user_id = ?", providerKey, userInfo.ProviderUserID).First(&existingIdentity).Error
	if err == nil {
		if existingIdentity.UserID != userID {
			s.logLogin(providerKey, userID, userInfo.ProviderUserID, userInfo.Email, "failed", "identity already linked to another user", ipAddress, userAgent)
			return nil, fmt.Errorf("this identity is already linked to another account")
		}
		s.logLogin(providerKey, userID, userInfo.ProviderUserID, userInfo.Email, "failed", "identity already linked", ipAddress, userAgent)
		return nil, fmt.Errorf("this identity is already linked to your account")
	}

	// 创建身份关联
	_, err = s.createIdentity(userID, providerKey, userInfo, token, false)
	if err != nil {
		s.logLogin(providerKey, userID, userInfo.ProviderUserID, userInfo.Email, "failed", "link failed: "+err.Error(), ipAddress, userAgent)
		return nil, err
	}

	var user models.User
	s.db.Where("user_id = ?", userID).First(&user)

	s.logLogin(providerKey, userID, userInfo.ProviderUserID, userInfo.Email, "user_linked", "", ipAddress, userAgent)

	return &LoginResult{User: &user, IsLinked: true}, nil
}

// ============================================
// 用户身份管理
// ============================================

// GetUserIdentities 获取用户绑定的所有身份
func (s *SSOService) GetUserIdentities(userID string) ([]models.UserIdentity, error) {
	var identities []models.UserIdentity
	err := s.db.Where("user_id = ?", userID).Order("is_primary DESC, created_at ASC").Find(&identities).Error
	return identities, err
}

// UnlinkIdentity 解绑身份
func (s *SSOService) UnlinkIdentity(userID string, identityID int64) error {
	var identity models.UserIdentity
	if err := s.db.First(&identity, identityID).Error; err != nil {
		return fmt.Errorf("identity not found")
	}

	if identity.UserID != userID {
		return fmt.Errorf("identity does not belong to this user")
	}

	// 检查是否为唯一登录方式
	var count int64
	s.db.Model(&models.UserIdentity{}).Where("user_id = ?", userID).Count(&count)

	var user models.User
	s.db.Where("user_id = ?", userID).First(&user)

	if count == 1 && user.PasswordHash == "" {
		return fmt.Errorf("cannot unlink the only login method")
	}

	// 如果是主要方式且还有其他方式，设置其他方式为主要
	if identity.IsPrimary && count > 1 {
		var otherIdentity models.UserIdentity
		s.db.Where("user_id = ? AND id != ?", userID, identityID).First(&otherIdentity)
		s.db.Model(&otherIdentity).Update("is_primary", true)
	}

	// 如果解绑后没有 SSO 身份了，更新 users 表
	if count == 1 {
		s.db.Model(&user).Updates(map[string]interface{}{
			"oauth_provider": "",
			"oauth_id":       "",
			"is_sso_user":    false,
		})
	}

	return s.db.Delete(&identity).Error
}

// SetPrimaryIdentity 设置主要登录方式
func (s *SSOService) SetPrimaryIdentity(userID string, identityID int64) error {
	var identity models.UserIdentity
	if err := s.db.First(&identity, identityID).Error; err != nil {
		return fmt.Errorf("identity not found")
	}

	if identity.UserID != userID {
		return fmt.Errorf("identity does not belong to this user")
	}

	// 取消当前主要方式
	s.db.Model(&models.UserIdentity{}).Where("user_id = ? AND is_primary = ?", userID, true).Update("is_primary", false)

	// 设置新的主要方式
	s.db.Model(&identity).Update("is_primary", true)

	// 更新 users 表的 OAuth 字段
	s.db.Model(&models.User{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
		"oauth_provider": identity.Provider,
		"oauth_id":       identity.ProviderUserID,
	})

	return nil
}

// ============================================
// 登录日志
// ============================================

// GetLoginLogs 获取 SSO 登录日志
func (s *SSOService) GetLoginLogs(page, pageSize int, providerKey string) ([]models.SSOLoginLog, int64, error) {
	var logs []models.SSOLoginLog
	var total int64

	query := s.db.Model(&models.SSOLoginLog{})
	if providerKey != "" {
		query = query.Where("provider_key = ?", providerKey)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}

// ============================================
// 内部辅助方法
// ============================================

// createSSOUser 创建 SSO 用户
func (s *SSOService) createSSOUser(providerKey string, providerCfg *models.SSOProvider, userInfo *StandardUserInfo) (*models.User, error) {
	// 生成用户名（使用 Provider 名称 + 用户名/邮箱前缀）
	username := generateSSOUsername(userInfo)

	// 确保用户名唯一
	username = s.ensureUniqueUsername(username)

	user := &models.User{
		Username:      username,
		Email:         userInfo.Email,
		PasswordHash:  "", // SSO 用户无密码
		IsActive:      true,
		IsSSOUser:     true,
		OAuthProvider: providerKey,
		OAuthID:       userInfo.ProviderUserID,
	}

	if err := s.db.Create(user).Error; err != nil {
		return nil, err
	}

	return user, nil
}

// createIdentity 创建身份关联
func (s *SSOService) createIdentity(userID string, providerKey string, userInfo *StandardUserInfo, token *OAuthToken, isPrimary bool) (*models.UserIdentity, error) {
	rawJSON, _ := json.Marshal(userInfo.RawData)

	identity := &models.UserIdentity{
		UserID:         userID,
		Provider:       providerKey,
		ProviderUserID: userInfo.ProviderUserID,
		ProviderEmail:  userInfo.Email,
		ProviderName:   userInfo.Name,
		ProviderAvatar: userInfo.Avatar,
		RawData:        rawJSON,
		IsPrimary:      isPrimary,
		IsVerified:     userInfo.EmailVerified,
	}

	now := time.Now()
	identity.LastUsedAt = &now

	if err := s.db.Create(identity).Error; err != nil {
		return nil, err
	}

	// 保存加密 token
	s.saveTokens(identity, token)

	return identity, nil
}

// saveTokens 加密保存 OAuth Token
func (s *SSOService) saveTokens(identity *models.UserIdentity, token *OAuthToken) {
	updates := map[string]interface{}{}

	if token.AccessToken != "" {
		if encrypted, err := crypto.EncryptValue(token.AccessToken); err == nil {
			updates["access_token_encrypted"] = encrypted
		}
	}
	if token.RefreshToken != "" {
		if encrypted, err := crypto.EncryptValue(token.RefreshToken); err == nil {
			updates["refresh_token_encrypted"] = encrypted
		}
	}
	if token.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
		updates["token_expires_at"] = expiresAt
	}

	if len(updates) > 0 {
		s.db.Model(identity).Updates(updates)
	}
}

// logLogin 记录 SSO 登录日志
func (s *SSOService) logLogin(providerKey, userID, providerUserID, providerEmail, status, errorMsg, ipAddress, userAgent string) {
	log := &models.SSOLoginLog{
		ProviderKey:    providerKey,
		UserID:         userID,
		ProviderUserID: providerUserID,
		ProviderEmail:  providerEmail,
		Status:         status,
		ErrorMessage:   errorMsg,
		IPAddress:      ipAddress,
		UserAgent:      userAgent,
	}
	s.db.Create(log)
}

// ============================================
// State 管理（CSRF 防护，使用 PostgreSQL 存储）
// ============================================

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// storeState 将 state 存储到数据库
func (s *SSOService) storeState(state string, data *StateData) {
	now := time.Now()
	ssoState := &models.SSOState{
		State:       state,
		ProviderKey: data.ProviderKey,
		RedirectURL: data.RedirectURL,
		Action:      data.Action,
		UserID:      data.UserID,
		CreatedAt:   now,
		ExpiresAt:   now.Add(10 * time.Minute),
	}
	s.db.Create(ssoState)
}

// validateState 从数据库验证并消费 state（一次性使用）
func (s *SSOService) validateState(state string) (*StateData, error) {
	var ssoState models.SSOState
	err := s.db.Where("state = ?", state).First(&ssoState).Error
	if err != nil {
		return nil, fmt.Errorf("state not found")
	}

	// 立即删除已使用的 state（一次性使用）
	s.db.Delete(&ssoState)

	// 检查是否过期
	if time.Now().After(ssoState.ExpiresAt) {
		return nil, fmt.Errorf("state expired")
	}

	return &StateData{
		ProviderKey: ssoState.ProviderKey,
		RedirectURL: ssoState.RedirectURL,
		Action:      ssoState.Action,
		UserID:      ssoState.UserID,
		CreatedAt:   ssoState.CreatedAt,
	}, nil
}

// cleanupExpiredStates 定期清理数据库中过期的 state
func (s *SSOService) cleanupExpiredStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.db.Where("expires_at < ?", time.Now()).Delete(&models.SSOState{})
	}
}

// ============================================
// SSO 全局配置
// ============================================

// SSOConfig SSO 全局配置
type SSOConfig struct {
	DisableLocalLogin bool `json:"disable_local_login"` // 禁用本地密码登录（超管例外）
}

// GetSSOConfig 获取 SSO 全局配置
func (s *SSOService) GetSSOConfig() *SSOConfig {
	config := &SSOConfig{
		DisableLocalLogin: false,
	}

	var sc models.SystemConfig
	if err := s.db.Where("key = ?", "sso_disable_local_login").First(&sc).Error; err == nil {
		if sc.Value == "true" {
			config.DisableLocalLogin = true
		}
	}

	return config
}

// UpdateSSOConfig 更新 SSO 全局配置
func (s *SSOService) UpdateSSOConfig(config *SSOConfig, updatedBy string) error {
	return s.upsertConfig("sso_disable_local_login", fmt.Sprintf("%v", config.DisableLocalLogin), "禁用本地密码登录（超管例外）", updatedBy)
}

// IsLocalLoginDisabled 检查本地登录是否被禁用
func (s *SSOService) IsLocalLoginDisabled() bool {
	var sc models.SystemConfig
	if err := s.db.Where("key = ?", "sso_disable_local_login").First(&sc).Error; err == nil {
		return sc.Value == "true"
	}
	return false
}

// upsertConfig 插入或更新配置
func (s *SSOService) upsertConfig(key, value, description, updatedBy string) error {
	var existing models.SystemConfig
	err := s.db.Where("key = ?", key).First(&existing).Error
	if err != nil {
		// 不存在，创建
		config := models.SystemConfig{
			Key:         key,
			Value:       value,
			Description: description,
		}
		return s.db.Create(&config).Error
	}
	// 存在，更新
	return s.db.Model(&existing).Updates(map[string]interface{}{
		"value":       value,
		"description": description,
	}).Error
}

// ============================================
// 工具函数
// ============================================

// generateSSOUsername 生成 SSO 用户名
func generateSSOUsername(userInfo *StandardUserInfo) string {
	if userInfo.Name != "" {
		// 移除空格和特殊字符
		name := strings.ReplaceAll(userInfo.Name, " ", "_")
		name = strings.ToLower(name)
		return name
	}
	if userInfo.Email != "" {
		parts := strings.Split(userInfo.Email, "@")
		return parts[0]
	}
	return "sso_user"
}

// ensureUniqueUsername 确保用户名唯一
func (s *SSOService) ensureUniqueUsername(username string) string {
	var count int64
	s.db.Model(&models.User{}).Where("username = ?", username).Count(&count)
	if count == 0 {
		return username
	}

	// 添加数字后缀
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_%d", username, i)
		s.db.Model(&models.User{}).Where("username = ?", candidate).Count(&count)
		if count == 0 {
			return candidate
		}
	}

	// 极端情况：使用时间戳
	return fmt.Sprintf("%s_%d", username, time.Now().UnixNano())
}

// isEmailDomainAllowed 验证邮箱域名
func isEmailDomainAllowed(email string, allowedDomains []string) bool {
	if len(allowedDomains) == 0 {
		return true
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(parts[1])
	for _, allowed := range allowedDomains {
		if strings.ToLower(allowed) == domain {
			return true
		}
	}
	return false
}
