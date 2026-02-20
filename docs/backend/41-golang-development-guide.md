# Go开发规范和测试指南

## 📋 目录
- [项目结构](#项目结构)
- [代码规范](#代码规范)
- [测试规范](#测试规范)
- [错误处理](#错误处理)
- [性能优化](#性能优化)
- [安全规范](#安全规范)

## 🏗️ 项目结构

### 标准目录结构
```
backend/
├── cmd/                    # 应用程序入口
├── internal/              # 私有代码
│   ├── config/           # 配置管理
│   ├── database/         # 数据库连接
│   ├── handlers/         # HTTP处理器
│   ├── middleware/       # 中间件
│   ├── models/          # 数据模型
│   └── router/          # 路由配置
├── controllers/          # 控制器层
├── services/            # 业务逻辑层
├── pkg/                 # 公共库
├── tests/              # 测试文件
├── docs/               # 文档
├── scripts/            # 脚本文件
├── go.mod              # Go模块文件
├── go.sum              # 依赖校验文件
├── Makefile           # 构建脚本
└── README.md          # 项目说明
```

## 📝 代码规范

### 命名规范
```go
// 包名：小写，简短，有意义
package controllers

// 常量：大写，下划线分隔
const (
    MAX_RETRY_COUNT = 3
    DEFAULT_TIMEOUT = 30
)

// 变量：驼峰命名
var userService *UserService

// 函数：驼峰命名，公开函数首字母大写
func GetUserByID(id uint) (*User, error) {}
func validateInput(input string) bool {}

// 结构体：驼峰命名，公开结构体首字母大写
type UserController struct {
    userService *UserService
}

// 接口：以er结尾
type UserRepository interface {
    Create(user *User) error
    GetByID(id uint) (*User, error)
}
```

### 代码组织
```go
package controllers

import (
    // 标准库
    "fmt"
    "net/http"
    "strconv"
    "time"
    
    // 第三方库
    "github.com/gin-gonic/gin"
    
    // 项目内部包
    "iac-platform/internal/models"
    "iac-platform/services"
)

// 常量定义
const (
    DefaultPageSize = 20
    MaxPageSize     = 100
)

// 类型定义
type Controller struct {
    service Service
}

// 构造函数
func NewController(service Service) *Controller {
    return &Controller{
        service: service,
    }
}

// 公开方法
func (c *Controller) GetList(ctx *gin.Context) {
    // 实现逻辑
}
```

## 🧪 测试规范

### 测试文件组织
```go
// 文件命名：*_test.go
// module_controller_test.go

package controllers

import (
    "testing"
    "net/http"
    "net/http/httptest"
    
    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)
```

### 单元测试
```go
func TestModuleController_GetModules(t *testing.T) {
    // 准备测试数据
    gin.SetMode(gin.TestMode)
    
    // 创建测试路由
    router := gin.New()
    controller := NewModuleController(mockService)
    router.GET("/modules", controller.GetModules)
    
    // 执行测试
    req, _ := http.NewRequest("GET", "/modules", nil)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)
    
    // 验证结果
    assert.Equal(t, http.StatusOK, w.Code)
    assert.Contains(t, w.Body.String(), "modules")
}
```

### 集成测试
```go
func TestModuleAPI_Integration(t *testing.T) {
    // 设置测试数据库
    db := setupTestDB()
    defer cleanupTestDB(db)
    
    // 创建测试服务器
    server := setupTestServer(db)
    defer server.Close()
    
    // 执行API测试
    resp, err := http.Get(server.URL + "/api/v1/modules")
    assert.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
}
```

### Mock测试
```go
type MockUserService struct {
    mock.Mock
}

func (m *MockUserService) GetByID(id uint) (*User, error) {
    args := m.Called(id)
    return args.Get(0).(*User), args.Error(1)
}

func TestUserController_GetUser(t *testing.T) {
    mockService := new(MockUserService)
    mockUser := &User{ID: 1, Name: "Test"}
    
    mockService.On("GetByID", uint(1)).Return(mockUser, nil)
    
    controller := NewUserController(mockService)
    // 执行测试...
    
    mockService.AssertExpectations(t)
}
```

### 基准测试
```go
func BenchmarkModuleController_GetModules(b *testing.B) {
    controller := setupController()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        controller.GetModules(mockContext)
    }
}
```

### 测试覆盖率
```bash
# 运行测试并生成覆盖率报告
go test -coverprofile=coverage.out ./...

# 查看覆盖率
go tool cover -html=coverage.out

# 设置覆盖率目标（80%以上）
go test -cover ./... | grep "coverage:"
```

##  错误处理

### 错误定义
```go
// 自定义错误类型
type APIError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Details string `json:"details,omitempty"`
}

func (e *APIError) Error() string {
    return e.Message
}

// 预定义错误
var (
    ErrUserNotFound = &APIError{
        Code:    404,
        Message: "用户不存在",
    }
    ErrInvalidInput = &APIError{
        Code:    400,
        Message: "输入参数无效",
    }
)
```

### 错误处理模式
```go
func (c *Controller) GetUser(ctx *gin.Context) {
    id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{
            "code":    400,
            "message": "无效的用户ID",
            "timestamp": time.Now().Format(time.RFC3339),
        })
        return
    }
    
    user, err := c.service.GetByID(uint(id))
    if err != nil {
        if errors.Is(err, ErrUserNotFound) {
            ctx.JSON(http.StatusNotFound, gin.H{
                "code":    404,
                "message": "用户不存在",
                "timestamp": time.Now().Format(time.RFC3339),
            })
            return
        }
        
        // 记录内部错误
        log.Printf("Internal error: %v", err)
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "code":    500,
            "message": "内部服务器错误",
            "timestamp": time.Now().Format(time.RFC3339),
        })
        return
    }
    
    ctx.JSON(http.StatusOK, gin.H{
        "code": 200,
        "data": user,
        "timestamp": time.Now().Format(time.RFC3339),
    })
}
```

## 🚀 性能优化

### 数据库优化
```go
// 使用连接池
func setupDB() *gorm.DB {
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        panic(err)
    }
    
    sqlDB, _ := db.DB()
    sqlDB.SetMaxIdleConns(10)
    sqlDB.SetMaxOpenConns(100)
    sqlDB.SetConnMaxLifetime(time.Hour)
    
    return db
}

// 预加载关联数据
func (s *UserService) GetUsersWithRoles() ([]User, error) {
    var users []User
    err := s.db.Preload("Roles").Find(&users).Error
    return users, err
}

// 批量操作
func (s *UserService) CreateUsers(users []User) error {
    return s.db.CreateInBatches(users, 100).Error
}
```

### 缓存策略
```go
type CacheService struct {
    cache map[string]interface{}
    mutex sync.RWMutex
}

func (c *CacheService) Get(key string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    
    value, exists := c.cache[key]
    return value, exists
}

func (c *CacheService) Set(key string, value interface{}) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    c.cache[key] = value
}
```

## 🔒 安全规范

### 输入验证
```go
func validateCreateUserRequest(req *CreateUserRequest) error {
    if req.Username == "" {
        return errors.New("用户名不能为空")
    }
    
    if len(req.Password) < 8 {
        return errors.New("密码长度不能少于8位")
    }
    
    emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
    if !emailRegex.MatchString(req.Email) {
        return errors.New("邮箱格式无效")
    }
    
    return nil
}
```

### SQL注入防护
```go
// 使用参数化查询
func (s *UserService) GetUserByEmail(email string) (*User, error) {
    var user User
    err := s.db.Where("email = ?", email).First(&user).Error
    return &user, err
}

// 避免直接拼接SQL
// 错误示例：
// query := fmt.Sprintf("SELECT * FROM users WHERE email = '%s'", email)

// 正确示例：
// s.db.Raw("SELECT * FROM users WHERE email = ?", email).Scan(&user)
```

### 认证和授权
```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        token := c.GetHeader("Authorization")
        if token == "" {
            c.JSON(http.StatusUnauthorized, gin.H{
                "code": 401,
                "message": "缺少认证令牌",
            })
            c.Abort()
            return
        }
        
        claims, err := validateJWT(token)
        if err != nil {
            c.JSON(http.StatusUnauthorized, gin.H{
                "code": 401,
                "message": "无效的认证令牌",
            })
            c.Abort()
            return
        }
        
        c.Set("user_id", claims.UserID)
        c.Next()
    }
}
```

## 📊 测试命令

### 基本测试命令
```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./controllers

# 运行特定测试函数
go test -run TestModuleController_GetModules

# 详细输出
go test -v ./...

# 并行测试
go test -parallel 4 ./...

# 测试覆盖率
go test -cover ./...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# 基准测试
go test -bench=. ./...

# 内存分析
go test -memprofile=mem.prof ./...
go tool pprof mem.prof
```

### Makefile测试目标
```makefile
# 测试相关命令
.PHONY: test test-cover test-bench test-integration

test:
	go test -v ./...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-bench:
	go test -bench=. -benchmem ./...

test-integration:
	go test -tags=integration ./tests/...

test-all: test test-cover test-bench
	@echo "All tests completed"
```

## 🔧 开发工具

### 推荐工具
- **IDE**: VS Code + Go扩展
- **代码格式化**: gofmt, goimports
- **代码检查**: golint, go vet, golangci-lint
- **测试工具**: testify, gomock
- **性能分析**: pprof
- **依赖管理**: go mod

### 代码质量检查
```bash
# 格式化代码
go fmt ./...

# 导入整理
goimports -w .

# 代码检查
go vet ./...

# 静态分析
golangci-lint run

# 安全检查
gosec ./...
```

## 📋 最佳实践

1. **测试驱动开发(TDD)**: 先写测试，再写实现
2. **单一职责**: 每个函数只做一件事
3. **依赖注入**: 便于测试和维护
4. **接口设计**: 面向接口编程
5. **错误处理**: 明确的错误类型和处理
6. **文档注释**: 公开API必须有注释
7. **性能监控**: 关键路径添加监控
8. **安全第一**: 输入验证和权限检查