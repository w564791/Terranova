package services

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"gorm.io/gorm"
)

// MFAService MFA服务
type MFAService struct {
	db *gorm.DB
}

// NewMFAService 创建MFA服务实例
func NewMFAService(db *gorm.DB) *MFAService {
	return &MFAService{db: db}
}

// GetMFAConfig 获取MFA全局配置
func (s *MFAService) GetMFAConfig() (*models.MFAConfig, error) {
	config := &models.MFAConfig{
		Enabled:                true,
		Enforcement:            "optional",
		Issuer:                 "IaC Platform",
		GracePeriodDays:        7,
		MaxFailedAttempts:      5,
		LockoutDurationMinutes: 15,
	}

	// 从数据库加载配置
	var configs []models.SystemConfig
	if err := s.db.Where("key LIKE ?", "mfa_%").Find(&configs).Error; err != nil {
		return config, err
	}

	for _, cfg := range configs {
		value := s.parseJSONString(cfg.Value)
		switch cfg.Key {
		case "mfa_enabled":
			config.Enabled = value == "true"
		case "mfa_enforcement":
			config.Enforcement = value
		case "mfa_enforcement_enabled_at":
			if value != "" && value != "null" {
				t, err := time.Parse(time.RFC3339, value)
				if err == nil {
					config.EnforcementEnabledAt = &t
				}
			}
		case "mfa_issuer":
			if value != "" {
				config.Issuer = value
			}
		case "mfa_grace_period_days":
			if v := s.parseJSONInt(cfg.Value); v > 0 {
				config.GracePeriodDays = v
			}
		case "mfa_max_failed_attempts":
			if v := s.parseJSONInt(cfg.Value); v > 0 {
				config.MaxFailedAttempts = v
			}
		case "mfa_lockout_duration_minutes":
			if v := s.parseJSONInt(cfg.Value); v > 0 {
				config.LockoutDurationMinutes = v
			}
		}
	}

	return config, nil
}

// UpdateMFAConfig 更新MFA全局配置
func (s *MFAService) UpdateMFAConfig(config *models.MFAConfig) error {
	updates := map[string]interface{}{
		"mfa_enabled":                  fmt.Sprintf("%t", config.Enabled),
		"mfa_enforcement":              fmt.Sprintf(`"%s"`, config.Enforcement),
		"mfa_issuer":                   fmt.Sprintf(`"%s"`, config.Issuer),
		"mfa_grace_period_days":        fmt.Sprintf("%d", config.GracePeriodDays),
		"mfa_max_failed_attempts":      fmt.Sprintf("%d", config.MaxFailedAttempts),
		"mfa_lockout_duration_minutes": fmt.Sprintf("%d", config.LockoutDurationMinutes),
	}

	// 如果策略从optional变为required_*，记录启用时间
	if config.Enforcement != "optional" && config.EnforcementEnabledAt == nil {
		now := time.Now()
		config.EnforcementEnabledAt = &now
		updates["mfa_enforcement_enabled_at"] = fmt.Sprintf(`"%s"`, now.Format(time.RFC3339))
	}

	for key, value := range updates {
		if err := s.upsertConfig(key, value.(string)); err != nil {
			return err
		}
	}

	return nil
}

// GetUserMFAStatus 获取用户MFA状态
func (s *MFAService) GetUserMFAStatus(user *models.User) (*models.MFAStatus, error) {
	config, err := s.GetMFAConfig()
	if err != nil {
		return nil, err
	}

	status := &models.MFAStatus{
		MFAEnabled:        user.MFAEnabled,
		MFAVerifiedAt:     user.MFAVerifiedAt,
		EnforcementPolicy: config.Enforcement,
		IsRequired:        s.isMFARequired(user, config),
		IsLocked:          s.isUserLocked(user),
		LockedUntil:       user.MFALockedUntil,
	}

	// 计算剩余备用恢复码数量
	if user.MFABackupCodes != "" {
		codes, err := s.decryptBackupCodes(user.MFABackupCodes)
		if err == nil {
			count := 0
			for _, code := range codes {
				if !code.Used {
					count++
				}
			}
			status.BackupCodesCount = count
		}
	}

	return status, nil
}

// SetupMFA 初始化MFA设置，生成密钥和二维码
func (s *MFAService) SetupMFA(user *models.User) (*models.MFASetupResponse, error) {
	config, err := s.GetMFAConfig()
	if err != nil {
		return nil, err
	}

	// 生成TOTP密钥
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      config.Issuer,
		AccountName: user.Username,
		Period:      30,
		SecretSize:  20,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	// 生成二维码
	qrCode, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}
	qrCodeBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrCode)

	// 生成备用恢复码
	backupCodes, err := s.generateBackupCodes(10)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// 加密并保存密钥（暂时保存，等待验证后启用）
	encryptedSecret, err := crypto.EncryptValue(key.Secret())
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// 加密备用恢复码
	backupCodesData := make([]models.MFABackupCode, len(backupCodes))
	for i, code := range backupCodes {
		backupCodesData[i] = models.MFABackupCode{Code: code, Used: false}
	}
	encryptedBackupCodes, err := s.encryptBackupCodes(backupCodesData)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt backup codes: %w", err)
	}

	// 更新用户记录（MFA尚未启用，等待验证）
	if err := s.db.Model(user).Updates(map[string]interface{}{
		"mfa_secret":       encryptedSecret,
		"mfa_backup_codes": encryptedBackupCodes,
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to save MFA secret: %w", err)
	}

	return &models.MFASetupResponse{
		Secret:      key.Secret(),
		QRCode:      qrCodeBase64,
		OTPAuthURI:  key.URL(),
		BackupCodes: backupCodes,
	}, nil
}

// VerifyAndEnableMFA 验证TOTP码并启用MFA
func (s *MFAService) VerifyAndEnableMFA(user *models.User, code string) error {
	if user.MFAEnabled {
		return fmt.Errorf("MFA is already enabled")
	}

	if user.MFASecret == "" {
		return fmt.Errorf("MFA setup not initiated")
	}

	// 解密密钥
	secret, err := crypto.DecryptValue(user.MFASecret)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret: %w", err)
	}

	// 验证TOTP码
	valid := totp.Validate(code, secret)
	if !valid {
		return fmt.Errorf("invalid verification code")
	}

	// 启用MFA
	now := time.Now()
	if err := s.db.Model(user).Updates(map[string]interface{}{
		"mfa_enabled":     true,
		"mfa_verified_at": now,
	}).Error; err != nil {
		return fmt.Errorf("failed to enable MFA: %w", err)
	}

	user.MFAEnabled = true
	user.MFAVerifiedAt = &now

	return nil
}

// VerifyMFACode 验证MFA码（登录时使用）
func (s *MFAService) VerifyMFACode(user *models.User, code string) error {
	config, err := s.GetMFAConfig()
	if err != nil {
		return err
	}

	// 检查是否被锁定
	if s.isUserLocked(user) {
		return fmt.Errorf("account is locked due to too many failed attempts")
	}

	// 解密密钥
	secret, err := crypto.DecryptValue(user.MFASecret)
	if err != nil {
		fmt.Printf("[MFA Debug] Failed to decrypt secret for user %s: %v\n", user.Username, err)
		return fmt.Errorf("failed to decrypt secret: %w", err)
	}

	fmt.Printf("[MFA Debug] User: %s, Code: %s, Secret length: %d\n", user.Username, code, len(secret))

	// 验证TOTP码 - 使用更宽松的时间窗口
	valid := totp.Validate(code, secret)
	if !valid {
		// 尝试使用ValidateCustom进行更详细的验证
		validCustom, err := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
			Period:    30,
			Skew:      1, // 允许前后各1个时间窗口
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		})
		fmt.Printf("[MFA Debug] ValidateCustom result: %v, err: %v\n", validCustom, err)

		if !validCustom {
			// 生成当前应该的验证码用于调试
			expectedCode, _ := totp.GenerateCode(secret, time.Now())
			fmt.Printf("[MFA Debug] Expected code: %s, Received code: %s, Server time: %s\n",
				expectedCode, code, time.Now().Format(time.RFC3339))

			// 增加失败次数
			s.incrementFailedAttempts(user, config)
			return fmt.Errorf("invalid verification code")
		}
	}

	// 重置失败次数
	s.resetFailedAttempts(user)

	return nil
}

// VerifyBackupCode 验证备用恢复码
func (s *MFAService) VerifyBackupCode(user *models.User, code string) error {
	config, err := s.GetMFAConfig()
	if err != nil {
		return err
	}

	// 检查是否被锁定
	if s.isUserLocked(user) {
		return fmt.Errorf("account is locked due to too many failed attempts")
	}

	// 解密备用恢复码
	codes, err := s.decryptBackupCodes(user.MFABackupCodes)
	if err != nil {
		return fmt.Errorf("failed to decrypt backup codes: %w", err)
	}

	// 查找并验证恢复码
	found := false
	for i, c := range codes {
		if c.Code == code && !c.Used {
			codes[i].Used = true
			now := time.Now()
			codes[i].UsedAt = &now
			found = true
			break
		}
	}

	if !found {
		// 增加失败次数
		s.incrementFailedAttempts(user, config)
		return fmt.Errorf("invalid or already used backup code")
	}

	// 保存更新后的备用恢复码
	encryptedCodes, err := s.encryptBackupCodes(codes)
	if err != nil {
		return fmt.Errorf("failed to encrypt backup codes: %w", err)
	}

	if err := s.db.Model(user).Update("mfa_backup_codes", encryptedCodes).Error; err != nil {
		return fmt.Errorf("failed to update backup codes: %w", err)
	}

	// 重置失败次数
	s.resetFailedAttempts(user)

	return nil
}

// DisableMFA 禁用MFA
func (s *MFAService) DisableMFA(user *models.User, code, password string) error {
	config, err := s.GetMFAConfig()
	if err != nil {
		return err
	}

	// 检查是否允许禁用
	if config.Enforcement == "required_all" {
		return fmt.Errorf("MFA cannot be disabled due to security policy")
	}

	// 验证TOTP码
	if err := s.VerifyMFACode(user, code); err != nil {
		return err
	}

	// 清除MFA设置
	if err := s.db.Model(user).Updates(map[string]interface{}{
		"mfa_enabled":         false,
		"mfa_secret":          "",
		"mfa_verified_at":     nil,
		"mfa_backup_codes":    "",
		"mfa_failed_attempts": 0,
		"mfa_locked_until":    nil,
	}).Error; err != nil {
		return fmt.Errorf("failed to disable MFA: %w", err)
	}

	return nil
}

// RegenerateBackupCodes 重新生成备用恢复码
func (s *MFAService) RegenerateBackupCodes(user *models.User, code string) ([]string, error) {
	// 验证TOTP码
	if err := s.VerifyMFACode(user, code); err != nil {
		return nil, err
	}

	// 生成新的备用恢复码
	backupCodes, err := s.generateBackupCodes(10)
	if err != nil {
		return nil, fmt.Errorf("failed to generate backup codes: %w", err)
	}

	// 加密备用恢复码
	backupCodesData := make([]models.MFABackupCode, len(backupCodes))
	for i, code := range backupCodes {
		backupCodesData[i] = models.MFABackupCode{Code: code, Used: false}
	}
	encryptedBackupCodes, err := s.encryptBackupCodes(backupCodesData)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt backup codes: %w", err)
	}

	// 保存新的备用恢复码
	if err := s.db.Model(user).Update("mfa_backup_codes", encryptedBackupCodes).Error; err != nil {
		return nil, fmt.Errorf("failed to save backup codes: %w", err)
	}

	return backupCodes, nil
}

// ResetUserMFA 管理员重置用户MFA
func (s *MFAService) ResetUserMFA(userID string) error {
	if err := s.db.Model(&models.User{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
		"mfa_enabled":         false,
		"mfa_secret":          "",
		"mfa_verified_at":     nil,
		"mfa_backup_codes":    "",
		"mfa_failed_attempts": 0,
		"mfa_locked_until":    nil,
	}).Error; err != nil {
		return fmt.Errorf("failed to reset MFA: %w", err)
	}

	return nil
}

// CreateMFAToken 创建MFA临时令牌
func (s *MFAService) CreateMFAToken(userID, ipAddress string) (*models.MFAToken, error) {
	// 生成随机令牌
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	token := base32.StdEncoding.EncodeToString(tokenBytes)[:32]

	mfaToken := &models.MFAToken{
		Token:     token,
		UserID:    userID,
		IPAddress: ipAddress,
		ExpiresAt: time.Now().Add(10 * time.Minute), // 延长到10分钟
	}

	if err := s.db.Create(mfaToken).Error; err != nil {
		return nil, fmt.Errorf("failed to create MFA token: %w", err)
	}

	return mfaToken, nil
}

// ValidateMFAToken 验证MFA临时令牌
func (s *MFAService) ValidateMFAToken(token, ipAddress string) (*models.MFAToken, error) {
	fmt.Printf("[MFA Debug] ValidateMFAToken called with token: %s, IP: %s\n", token[:min(10, len(token))]+"...", ipAddress)

	var mfaToken models.MFAToken
	if err := s.db.Where("token = ?", token).First(&mfaToken).Error; err != nil {
		fmt.Printf("[MFA Debug] Token not found in database: %v\n", err)
		return nil, fmt.Errorf("invalid MFA token")
	}

	fmt.Printf("[MFA Debug] Token found: UserID=%s, ExpiresAt=%s, Used=%v, IPAddress=%s\n",
		mfaToken.UserID, mfaToken.ExpiresAt.Format(time.RFC3339), mfaToken.Used, mfaToken.IPAddress)

	if !mfaToken.IsValid() {
		fmt.Printf("[MFA Debug] Token is invalid (expired or used)\n")
		return nil, fmt.Errorf("MFA token is expired or already used")
	}

	// 验证IP地址（可选，增强安全性）
	if mfaToken.IPAddress != "" && mfaToken.IPAddress != ipAddress {
		fmt.Printf("[MFA Debug] IP mismatch: expected %s, got %s\n", mfaToken.IPAddress, ipAddress)
		return nil, fmt.Errorf("IP address mismatch")
	}

	fmt.Printf("[MFA Debug] Token validation successful\n")
	return &mfaToken, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// MarkMFATokenUsed 标记MFA令牌为已使用
func (s *MFAService) MarkMFATokenUsed(token *models.MFAToken) error {
	now := time.Now()
	return s.db.Model(token).Updates(map[string]interface{}{
		"used":    true,
		"used_at": now,
	}).Error
}

// GetMFAStatistics 获取MFA使用统计
func (s *MFAService) GetMFAStatistics() (*models.MFAStatistics, error) {
	var stats models.MFAStatistics
	var totalCount, enabledCount int64

	// 总用户数
	if err := s.db.Model(&models.User{}).Where("is_active = ?", true).Count(&totalCount).Error; err != nil {
		return nil, err
	}
	stats.TotalUsers = int(totalCount)

	// 已启用MFA的用户数
	if err := s.db.Model(&models.User{}).Where("is_active = ? AND mfa_enabled = ?", true, true).Count(&enabledCount).Error; err != nil {
		return nil, err
	}
	stats.MFAEnabledUsers = int(enabledCount)

	stats.MFAPendingUsers = stats.TotalUsers - stats.MFAEnabledUsers

	return &stats, nil
}

// 辅助方法

func (s *MFAService) isMFARequired(user *models.User, config *models.MFAConfig) bool {
	if !config.Enabled {
		return false
	}

	switch config.Enforcement {
	case "required_all":
		// 检查宽限期
		if config.EnforcementEnabledAt != nil {
			gracePeriodEnd := config.EnforcementEnabledAt.AddDate(0, 0, config.GracePeriodDays)
			if time.Now().Before(gracePeriodEnd) {
				return false // 在宽限期内
			}
		}
		return true
	case "required_new":
		// 新用户必须设置MFA
		if config.EnforcementEnabledAt != nil && user.CreatedAt.After(*config.EnforcementEnabledAt) {
			return true
		}
		return false
	default:
		return false
	}
}

func (s *MFAService) isUserLocked(user *models.User) bool {
	if user.MFALockedUntil == nil {
		return false
	}
	return time.Now().Before(*user.MFALockedUntil)
}

func (s *MFAService) incrementFailedAttempts(user *models.User, config *models.MFAConfig) {
	user.MFAFailedAttempts++

	updates := map[string]interface{}{
		"mfa_failed_attempts": user.MFAFailedAttempts,
	}

	if user.MFAFailedAttempts >= config.MaxFailedAttempts {
		lockUntil := time.Now().Add(time.Duration(config.LockoutDurationMinutes) * time.Minute)
		user.MFALockedUntil = &lockUntil
		updates["mfa_locked_until"] = lockUntil
	}

	s.db.Model(user).Updates(updates)
}

func (s *MFAService) resetFailedAttempts(user *models.User) {
	s.db.Model(user).Updates(map[string]interface{}{
		"mfa_failed_attempts": 0,
		"mfa_locked_until":    nil,
	})
}

func (s *MFAService) generateBackupCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		bytes := make([]byte, 4)
		if _, err := rand.Read(bytes); err != nil {
			return nil, err
		}
		// 生成8位数字恢复码
		code := fmt.Sprintf("%08d", (int(bytes[0])<<24|int(bytes[1])<<16|int(bytes[2])<<8|int(bytes[3]))%100000000)
		codes[i] = code
	}
	return codes, nil
}

func (s *MFAService) encryptBackupCodes(codes []models.MFABackupCode) (string, error) {
	data, err := json.Marshal(codes)
	if err != nil {
		return "", err
	}
	return crypto.EncryptValue(string(data))
}

func (s *MFAService) decryptBackupCodes(encrypted string) ([]models.MFABackupCode, error) {
	decrypted, err := crypto.DecryptValue(encrypted)
	if err != nil {
		return nil, err
	}

	var codes []models.MFABackupCode
	if err := json.Unmarshal([]byte(decrypted), &codes); err != nil {
		return nil, err
	}

	return codes, nil
}

func (s *MFAService) parseJSONString(value interface{}) string {
	if value == nil {
		return ""
	}

	str, ok := value.(string)
	if !ok {
		return ""
	}

	// 移除引号
	str = strings.Trim(str, `"`)
	return str
}

func (s *MFAService) parseJSONInt(value interface{}) int {
	if value == nil {
		return 0
	}

	str, ok := value.(string)
	if !ok {
		return 0
	}

	var v int
	fmt.Sscanf(str, "%d", &v)
	return v
}

func (s *MFAService) upsertConfig(key, value string) error {
	var existing models.SystemConfig
	err := s.db.Where("key = ?", key).First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		config := models.SystemConfig{
			Key:       key,
			Value:     value,
			UpdatedAt: time.Now(),
		}
		return s.db.Create(&config).Error
	}

	if err != nil {
		return err
	}

	existing.Value = value
	existing.UpdatedAt = time.Now()
	return s.db.Save(&existing).Error
}
