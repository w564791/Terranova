package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"

	"iac-platform/internal/config"
)

var encryptionKey []byte

func init() {
	// 使用JWT_SECRET作为加密密钥
	// 通过SHA256哈希确保密钥长度为32字节（AES-256要求）
	jwtSecret := config.GetJWTSecret()
	hash := sha256.Sum256([]byte(jwtSecret))
	encryptionKey = hash[:]
}

// EncryptValue 加密变量值
func EncryptValue(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptValue 解密变量值
func DecryptValue(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		// 如果base64解码失败，可能是明文（向后兼容）
		return ciphertext, nil
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		// 数据太短，可能是明文（向后兼容）
		return ciphertext, nil
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		// 解密失败，可能是明文（向后兼容）
		return ciphertext, nil
	}

	return string(plaintext), nil
}

// IsEncrypted 检查值是否已加密
func IsEncrypted(value string) bool {
	if value == "" {
		return false
	}

	// 尝试base64解码
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}

	// 检查长度是否合理（至少包含nonce）
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return false
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return false
	}

	return len(data) >= gcm.NonceSize()
}
