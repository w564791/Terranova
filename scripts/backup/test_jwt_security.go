package main

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	// 测试场景1: 伪造一个login token，尝试冒充admin
	fmt.Println("=== 测试场景1: 伪造login token冒充admin ===")
	fakeLoginToken := generateFakeLoginToken("user-n8tzt0ldde", "admin", "admin")
	fmt.Println("伪造的login token:")
	fmt.Println(fakeLoginToken)
	fmt.Println()

	// 测试场景2: 伪造一个user token，使用不存在的token_id
	fmt.Println("=== 测试场景2: 伪造user token使用不存在的token_id ===")
	fakeUserToken := generateFakeUserToken("user-n8tzt0ldde", "admin", "fake-token-id")
	fmt.Println("伪造的user token:")
	fmt.Println(fakeUserToken)
	fmt.Println()

	// 测试场景3: 伪造一个user token，使用其他用户的user_id
	fmt.Println("=== 测试场景3: 伪造user token使用其他用户的user_id ===")
	fakeUserToken2 := generateFakeUserToken("user-other-user", "hacker", "fake-token-id-2")
	fmt.Println("伪造的user token (其他用户):")
	fmt.Println(fakeUserToken2)
	fmt.Println()

	fmt.Println("=== 测试命令 ===")
	fmt.Println("测试场景1 (应该成功，因为login token不验证数据库):")
	fmt.Printf("curl -X GET 'http://localhost:8080/api/v1/iam/roles?is_active=true' -H 'Authorization: Bearer %s'\n\n", fakeLoginToken)

	fmt.Println("测试场景2 (应该失败，因为token_id不存在):")
	fmt.Printf("curl -X GET 'http://localhost:8080/api/v1/iam/roles?is_active=true' -H 'Authorization: Bearer %s'\n\n", fakeUserToken)

	fmt.Println("测试场景3 (应该失败，因为token_id不存在):")
	fmt.Printf("curl -X GET 'http://localhost:8080/api/v1/iam/roles?is_active=true' -H 'Authorization: Bearer %s'\n\n", fakeUserToken2)
}

func generateFakeLoginToken(userID, username, role string) string {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"role":     role,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("your-jwt-secret-key"))
	return tokenString
}

func generateFakeUserToken(userID, username, tokenID string) string {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"token_id": tokenID,
		"type":     "user_token",
		"exp":      time.Now().Add(365 * 24 * time.Hour).Unix(),
		"nbf":      time.Now().Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("your-jwt-secret-key"))
	return tokenString
}
