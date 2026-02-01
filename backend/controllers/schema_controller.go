package controllers

import (
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
)

type SchemaController struct {
	schemaService *services.SchemaService
}

func NewSchemaController(schemaService *services.SchemaService) *SchemaController {
	return &SchemaController{
		schemaService: schemaService,
	}
}

// GetSchemas 获取模块的Schema列表
// @Summary 获取模块Schema列表
// @Description 获取指定模块的Schema配置。如果不传 version_id，则返回默认版本的 Schema
// @Tags Schema
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param version_id query string false "Module Version ID (modv-xxx)，不传则获取默认版本的 Schema"
// @Success 200 {object} map[string]interface{} "成功返回Schema列表"
// @Failure 400 {object} map[string]interface{} "无效的模块ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/modules/{id}/schemas [get]
// @Security Bearer
func (c *SchemaController) GetSchemas(ctx *gin.Context) {
	moduleIDStr := ctx.Param("id")
	moduleID, err := strconv.ParseUint(moduleIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid module ID",
			"timestamp": time.Now(),
		})
		return
	}

	// 支持按 version_id 过滤
	versionID := ctx.Query("version_id")

	schemas, err := c.schemaService.GetSchemasByModuleIDAndVersion(uint(moduleID), versionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get schemas",
			"timestamp": time.Now(),
		})
		return
	}

	// 处理Schema数据，确保前端能正确解析
	processedSchemas, err := c.schemaService.ProcessSchemasForResponse(schemas)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to process schemas",
			"timestamp": time.Now(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Success",
		"data":      processedSchemas,
		"timestamp": time.Now(),
	})
}

// CreateSchema 创建Schema
// @Summary 创建模块Schema
// @Description 为指定模块创建新的Schema配置
// @Tags Schema
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param request body models.CreateSchemaRequest true "Schema信息"
// @Success 201 {object} map[string]interface{} "Schema创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/modules/{id}/schemas [post]
// @Security Bearer
func (c *SchemaController) CreateSchema(ctx *gin.Context) {
	moduleIDStr := ctx.Param("id")
	moduleID, err := strconv.ParseUint(moduleIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid module ID",
			"timestamp": time.Now(),
		})
		return
	}

	var req models.CreateSchemaRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	schema, err := c.schemaService.CreateSchema(uint(moduleID), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to create schema",
			"timestamp": time.Now(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"code":      201,
		"message":   "Schema created successfully",
		"data":      schema,
		"timestamp": time.Now(),
	})
}

// GetSchema 获取Schema详情
// @Summary 获取Schema详情
// @Description 根据ID获取Schema的详细信息
// @Tags Schema
// @Accept json
// @Produce json
// @Param id path int true "Schema ID"
// @Success 200 {object} map[string]interface{} "成功返回Schema详情"
// @Failure 400 {object} map[string]interface{} "无效的Schema ID"
// @Failure 404 {object} map[string]interface{} "Schema不存在"
// @Router /api/v1/schemas/{id} [get]
// @Security Bearer
func (c *SchemaController) GetSchema(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid schema ID",
			"timestamp": time.Now(),
		})
		return
	}

	schema, err := c.schemaService.GetSchemaByID(uint(id))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "Schema not found",
			"timestamp": time.Now(),
		})
		return
	}

	// 处理Schema数据，确保前端能正确解析
	processedSchema, err := c.schemaService.ProcessSchemaForResponse(schema)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to process schema",
			"timestamp": time.Now(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Success",
		"data":      processedSchema,
		"timestamp": time.Now(),
	})
}

// UpdateSchema 更新Schema
// @Summary 更新Schema配置
// @Description 更新Schema的配置信息
// @Tags Schema
// @Accept json
// @Produce json
// @Param id path int true "Schema ID"
// @Param request body models.UpdateSchemaRequest true "更新信息"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/schemas/{id} [put]
// @Security Bearer
func (c *SchemaController) UpdateSchema(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid schema ID",
			"timestamp": time.Now(),
		})
		return
	}

	var req models.UpdateSchemaRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	schema, err := c.schemaService.UpdateSchema(uint(id), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to update schema",
			"timestamp": time.Now(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Schema updated successfully",
		"data":      schema,
		"timestamp": time.Now(),
	})
}

// GenerateSchemaFromModule AI解析Module生成Schema
// @Summary AI生成Schema
// @Description 使用AI解析模块文件自动生成Schema配置
// @Tags Schema
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Success 201 {object} map[string]interface{} "Schema生成成功"
// @Failure 400 {object} map[string]interface{} "无效的模块ID"
// @Failure 500 {object} map[string]interface{} "生成失败"
// @Router /api/v1/modules/{id}/schemas/generate [post]
// @Security Bearer
func (c *SchemaController) GenerateSchemaFromModule(ctx *gin.Context) {
	moduleIDStr := ctx.Param("id")
	moduleID, err := strconv.ParseUint(moduleIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid module ID",
			"timestamp": time.Now(),
		})
		return
	}

	schema, err := c.schemaService.GenerateSchemaFromModule(uint(moduleID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to generate schema: " + err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"code":      201,
		"message":   "Schema generated successfully",
		"data":      schema,
		"timestamp": time.Now(),
	})
}
