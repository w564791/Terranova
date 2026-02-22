package services

import (
	"iac-platform/internal/config"
	"iac-platform/internal/models"
	"log"
)

// GenerateConfigWithCMDBSkillWithProgress 使用 Skill 模式生成配置（带进度回调）
// 这是 GenerateConfigWithCMDBSkill 的带进度回调版本
func (s *AICMDBSkillService) GenerateConfigWithCMDBSkillWithProgress(
	userID string,
	moduleID uint,
	userDescription string,
	workspaceID string,
	organizationID string,
	userSelections map[string]interface{},
	currentConfig map[string]interface{},
	mode string,
	resourceInfoMap map[string]interface{},
	progressCallback ProgressCallback,
) (*GenerateConfigWithCMDBResponse, error) {
	totalTimer := NewTimer()
	totalSteps := 5

	// 已完成步骤列表
	var completedSteps []CompletedStep
	var lastStepTimer *Timer

	// 辅助函数：安全地推送进度（带已完成步骤列表）
	reportProgress := func(step int, stepName, message string) {
		// 如果有上一个步骤，记录其耗时
		if lastStepTimer != nil && step > 1 {
			// 上一个步骤已完成，记录耗时
			if len(completedSteps) < step-1 {
				// 获取上一个步骤的名称（从上一次调用中获取）
				prevStepName := ""
				switch step - 1 {
				case 1:
					prevStepName = "初始化"
				case 2:
					prevStepName = "意图断言"
				case 3:
					prevStepName = "CMDB查询"
				case 4:
					prevStepName = "组装Prompt"
				case 5:
					prevStepName = "AI生成"
				}
				completedSteps = append(completedSteps, CompletedStep{
					Name:      prevStepName,
					ElapsedMs: int64(lastStepTimer.ElapsedMs()),
				})
			}
		}
		// 开始新步骤的计时
		lastStepTimer = NewTimer()

		if progressCallback != nil {
			progressCallback(ProgressEvent{
				Type:           "progress",
				Step:           step,
				TotalSteps:     totalSteps,
				StepName:       stepName,
				Message:        message,
				CompletedSteps: completedSteps,
			})
		}
	}

	log.Printf("[AICMDBSkillService] ========== 开始 Skill 模式配置生成（带进度） ==========")

	// 步骤 1: 获取 AI 配置
	reportProgress(1, "初始化", "正在获取 AI 配置...")
	configTimer := NewTimer()
	aiConfig, err := s.configService.GetConfigForCapability("form_generation")
	if err != nil || aiConfig == nil {
		IncAICallCount("form_generation", "config_error")
		return nil, err
	}
	RecordAICallDuration("form_generation", "get_config", configTimer.ElapsedMs())

	// 转换 userSelections
	convertedSelections := s.convertUserSelections(userSelections)

	// 检查配置模式
	if aiConfig.Mode != "skill" {
		log.Printf("[AICMDBSkillService] AI 配置模式为 '%s'，降级到传统模式", aiConfig.Mode)
		return s.fallbackToLegacyMode(userID, moduleID, userDescription, workspaceID, organizationID, convertedSelections, currentConfig, mode)
	}

	// 获取 Skill 组合配置
	composition := &aiConfig.SkillComposition
	if len(composition.FoundationSkills) == 0 && composition.TaskSkill == "" {
		composition = s.getDefaultSkillComposition()
	}

	// 步骤 2: 意图断言
	reportProgress(2, "意图断言", "正在检查请求安全性...")
	assertionTimer := NewTimer()
	assertionResult, err := s.performIntentAssertion(userID, userDescription)
	RecordAICallDuration("form_generation", "intent_assertion", assertionTimer.ElapsedMs())
	if err != nil {
		log.Printf("[AICMDBSkillService] 意图断言服务不可用: %v，继续执行", err)
	} else if assertionResult != nil && !assertionResult.IsSafe {
		IncAICallCount("form_generation", "blocked")
		return &GenerateConfigWithCMDBResponse{
			Status:  "blocked",
			Message: assertionResult.Suggestion,
		}, nil
	}

	// 步骤 3: CMDB 查询
	var cmdbData string
	var cmdbLookups []CMDBLookupResult

	if len(convertedSelections) > 0 {
		reportProgress(3, "处理选择", "正在处理用户选择的资源...")
		cmdbData = s.buildCMDBDataFromSelections(convertedSelections)
	} else if s.shouldUseCMDB(userDescription) {
		reportProgress(3, "查询CMDB", "正在查询 CMDB 资源...")
		cmdbTimer := NewTimer()
		cmdbResults, err := s.performCMDBQuery(userID, userDescription, convertedSelections)
		RecordAICallDuration("form_generation", "cmdb_query", cmdbTimer.ElapsedMs())
		if err != nil {
			log.Printf("[AICMDBSkillService] CMDB 查询失败: %v", err)
		} else {
			needSelection, lookups := s.checkNeedSelection(cmdbResults)
			if needSelection {
				return &GenerateConfigWithCMDBResponse{
					Status:      "need_selection",
					CMDBLookups: lookups,
					Message:     "找到多个匹配的资源，请选择",
				}, nil
			}
			cmdbLookups = lookups
			cmdbData = s.buildCMDBDataString(cmdbResults)
		}
	} else {
		reportProgress(3, "跳过CMDB", "不需要 CMDB 查询")
	}

	// 步骤 4: 组装 Prompt
	reportProgress(4, "组装Skill", "正在组装 Skill 提示词...")
	// 【重要】移除 Schema 直接加载，AI 只使用 Module Skill
	// schemaData := s.getSchemaData(moduleID) // 已移除
	dynamicContext := &DynamicContext{
		UserDescription: userDescription,
		WorkspaceID:     workspaceID,
		OrganizationID:  organizationID,
		ModuleID:        moduleID,
		UseCMDB:         cmdbData != "",
		CurrentConfig:   currentConfig,
		CMDBData:        cmdbData,
		SchemaData:      "", // 不再传递 Schema 数据给 AI
		ExtraContext: map[string]interface{}{
			"mode": mode,
		},
	}

	assembleTimer := NewTimer()
	assembleResult, err := s.skillAssembler.AssemblePrompt(composition, moduleID, dynamicContext)
	RecordSkillAssemblyDuration("form_generation", len(assembleResult.UsedSkillNames), assembleTimer.ElapsedMs())
	if err != nil {
		return s.fallbackToLegacyMode(userID, moduleID, userDescription, workspaceID, organizationID, convertedSelections, currentConfig, mode)
	}

	// 步骤 5: AI 生成
	reportProgress(5, "AI生成", "正在调用 AI 生成配置...")
	aiTimer := NewTimer()
	aiResult, err := s.aiFormService.callAI(aiConfig, assembleResult.Prompt)
	RecordAICallDuration("form_generation", "ai_call", aiTimer.ElapsedMs())
	if err != nil {
		IncAICallCount("form_generation", "ai_error")
		return nil, err
	}

	// 记录最后一步的耗时
	completedSteps = append(completedSteps, CompletedStep{
		Name:      "AI生成",
		ElapsedMs: int64(aiTimer.ElapsedMs()),
	})

	// 发送最终进度（包含所有已完成步骤）
	if progressCallback != nil {
		progressCallback(ProgressEvent{
			Type:           "progress",
			Step:           5,
			TotalSteps:     totalSteps,
			StepName:       "完成",
			Message:        "配置生成完成",
			CompletedSteps: completedSteps,
		})
	}

	// 解析响应
	response, err := s.parseAIResponse(aiResult, moduleID)
	if err != nil {
		IncAICallCount("form_generation", "parse_error")
		return nil, err
	}

	response.CMDBLookups = cmdbLookups

	// 记录指标
	executionTimeMs := int(totalTimer.ElapsedMs())
	RecordAICallDuration("form_generation", "total", totalTimer.ElapsedMs())
	IncAICallCount("form_generation", "success")

	if err := s.skillAssembler.LogSkillUsage(
		assembleResult.UsedSkillIDs,
		"form_generation",
		workspaceID,
		userID,
		&moduleID,
		aiConfig.ModelID,
		executionTimeMs,
	); err != nil {
		log.Printf("[AICMDBSkillService] 记录 Skill 使用日志失败: %v", err)
	}

	log.Printf("[AICMDBSkillService] ========== Skill 模式配置生成完成 ==========")
	return response, nil
}

// GenerateConfigWithCMDBSkillOptimizedWithProgress 使用优化版 Skill 模式生成配置（带进度回调）
// 这是 GenerateConfigWithCMDBSkillOptimized 的带进度回调版本
func (s *AICMDBSkillService) GenerateConfigWithCMDBSkillOptimizedWithProgress(
	userID string,
	moduleID uint,
	userDescription string,
	workspaceID string,
	organizationID string,
	userSelections map[string]interface{},
	currentConfig map[string]interface{},
	mode string,
	resourceInfoMap map[string]interface{},
	progressCallback ProgressCallback,
) (*GenerateConfigWithCMDBResponse, error) {
	totalTimer := NewTimer()
	totalSteps := 5

	// 已完成步骤列表
	var completedSteps []CompletedStep
	var lastStepTimer *Timer

	// 辅助函数：安全地推送进度（带已完成步骤列表）
	reportProgress := func(step int, stepName, message string) {
		// 如果有上一个步骤，记录其耗时
		if lastStepTimer != nil && step > 1 {
			// 上一个步骤已完成，记录耗时
			if len(completedSteps) < step-1 {
				// 获取上一个步骤的名称
				prevStepName := ""
				switch step - 1 {
				case 1:
					prevStepName = "初始化"
				case 2:
					prevStepName = "意图断言"
				case 3:
					prevStepName = "CMDB查询+Skill选择"
				case 4:
					prevStepName = "组装Prompt"
				case 5:
					prevStepName = "AI生成"
				}
				completedSteps = append(completedSteps, CompletedStep{
					Name:      prevStepName,
					ElapsedMs: int64(lastStepTimer.ElapsedMs()),
				})
			}
		}
		// 开始新步骤的计时
		lastStepTimer = NewTimer()

		if progressCallback != nil {
			progressCallback(ProgressEvent{
				Type:           "progress",
				Step:           step,
				TotalSteps:     totalSteps,
				StepName:       stepName,
				Message:        message,
				CompletedSteps: completedSteps,
			})
		}
	}

	log.Printf("[AICMDBSkillService] ========== 开始优化版 Skill 模式配置生成（带进度） ==========")

	// 步骤 1: 获取 AI 配置
	reportProgress(1, "初始化", "正在获取 AI 配置...")
	configTimer := NewTimer()
	aiConfig, err := s.configService.GetConfigForCapability("form_generation")
	if err != nil || aiConfig == nil {
		IncAICallCount("form_generation_optimized", "config_error")
		return nil, err
	}
	RecordAICallDuration("form_generation_optimized", "get_config", configTimer.ElapsedMs())

	// 转换 userSelections
	convertedSelections := s.convertUserSelections(userSelections)

	// 检查配置模式
	if aiConfig.Mode != "skill" {
		log.Printf("[AICMDBSkillService] AI 配置模式为 '%s'，降级到传统模式", aiConfig.Mode)
		IncAICallCount("form_generation_optimized", "fallback_legacy")
		return s.fallbackToLegacyMode(userID, moduleID, userDescription, workspaceID, organizationID, convertedSelections, currentConfig, mode)
	}

	// 步骤 2: 意图断言
	reportProgress(2, "意图断言", "正在检查请求安全性...")
	assertionTimer := NewTimer()
	assertionResult, err := s.performIntentAssertion(userID, userDescription)
	RecordAICallDuration("form_generation_optimized", "intent_assertion", assertionTimer.ElapsedMs())
	if err != nil {
		log.Printf("[AICMDBSkillService] 意图断言服务不可用: %v，继续执行", err)
	} else if assertionResult != nil && !assertionResult.IsSafe {
		IncAICallCount("form_generation_optimized", "blocked")
		return &GenerateConfigWithCMDBResponse{
			Status:  "blocked",
			Message: assertionResult.Suggestion,
		}, nil
	}

	// 步骤 3: 并行执行 CMDB 查询和 Skill 选择
	var cmdbData string
	var selectedSkills []string
	var cmdbUsedSkills []string // CMDB 查询阶段使用的 Skills（用于 UI 展示）

	// 判断是否是第二步流程（用户选择后）
	isSecondPhase := len(convertedSelections) > 0

	if isSecondPhase {
		// 第二步流程：处理用户选择的资源 + Skill 自动发现 + 组装 Skill（合并为一步）
		reportProgress(3, "处理选择+Skill", "正在处理用户选择的资源并发现 Skill...")
		parallelTimer := NewTimer()

		// 处理 CMDB 数据
		if len(resourceInfoMap) > 0 {
			cmdbData = s.buildCMDBDataFromResourceInfoMap(resourceInfoMap)
		} else {
			cmdbData = s.buildCMDBDataFromSelections(convertedSelections)
		}

		// 同时执行 Skill 自动发现（第二步：配置生成阶段，用户已选择资源）
		skillResult, err := s.selectDomainSkillsByAI(userDescription, "second")
		if err != nil {
			log.Printf("[AICMDBSkillService] 第二步 Skill 选择失败: %v，降级到标签匹配", err)
		} else {
			selectedSkills = skillResult
		}

		RecordAICallDuration("form_generation_optimized", "second_phase_processing", parallelTimer.ElapsedMs())

		// 记录步骤 3 的耗时（第二步流程中，步骤 3 和步骤 4 合并）
		// 同时记录 AI 选择的 Domain Skills（不包含 CMDB 相关的 skills，因为第二步没有 CMDB 查询）
		if lastStepTimer != nil && len(completedSteps) < 3 {
			completedSteps = append(completedSteps, CompletedStep{
				Name:       "处理选择+Skill",
				ElapsedMs:  int64(lastStepTimer.ElapsedMs()),
				UsedSkills: selectedSkills, // 只显示 AI 选择的 Domain Skills
			})
		}

		// 第二步流程跳过步骤 4 的进度报告，直接进入 generateWithCMDBDataAndSkillsWithProgress
		// 在 generateWithCMDBDataAndSkillsWithProgress 中，组装 Skill 的时间会被记录但不单独显示
		return s.generateWithCMDBDataAndSkillsWithProgress(
			userID, moduleID, userDescription, workspaceID, organizationID,
			aiConfig, cmdbData, selectedSkills, currentConfig, mode, totalTimer,
			progressCallback, totalSteps, completedSteps, true, // skipStep4Display = true
		)
	} else {
		reportProgress(3, "CMDB查询+Skill选择", "正在执行 CMDB 查询...")
		parallelTimer := NewTimer()
		SetActiveParallelTasks(1)
		parallelResult := s.executeParallel(userID, userDescription)
		SetActiveParallelTasks(0)
		RecordAICallDuration("form_generation_optimized", "parallel_execution", parallelTimer.ElapsedMs())

		// 提取 CMDB 查询阶段使用的 Skills
		cmdbUsedSkills = parallelResult.CMDBUsedSkills

		// 处理 CMDB 结果
		if parallelResult.CMDBError != nil {
			log.Printf("[AICMDBSkillService] CMDB 查询失败: %v，继续执行", parallelResult.CMDBError)
		} else if parallelResult.NeedSelection {
			// 记录 CMDB 查询步骤的耗时
			if lastStepTimer != nil && len(completedSteps) < 3 {
				completedSteps = append(completedSteps, CompletedStep{
					Name:       "CMDB查询",
					ElapsedMs:  int64(lastStepTimer.ElapsedMs()),
					UsedSkills: cmdbUsedSkills,
				})
			}
			// 发送 need_selection 事件（包含已完成步骤）
			if progressCallback != nil {
				progressCallback(ProgressEvent{
					Type:           "need_selection",
					Step:           0,
					TotalSteps:     0,
					StepName:       "需要选择",
					Message:        "找到多个匹配的资源，请选择",
					CompletedSteps: completedSteps,
				})
			}
			return &GenerateConfigWithCMDBResponse{
				Status:      "need_selection",
				CMDBLookups: parallelResult.CMDBLookups,
				Message:     "找到多个匹配的资源，请选择",
			}, nil
		} else if parallelResult.CMDBResults != nil {
			cmdbData = s.buildCMDBDataString(parallelResult.CMDBResults)
		}

		// 阶段二：根据用户描述 AI 选择 Domain Skills（用于资源生成）
		skillSelectionTimer := NewTimer()
		secondPhaseSkills, err := s.selectDomainSkillsByAI(userDescription, "second")
		RecordAICallDuration("form_generation_optimized", "second_phase_skill_selection", skillSelectionTimer.ElapsedMs())
		log.Printf("[AICMDBSkillService] [耗时] 阶段二 Skill 选择: %.0fms", skillSelectionTimer.ElapsedMs())
		if err != nil {
			log.Printf("[AICMDBSkillService] 阶段二 Skill 选择失败: %v，降级到组合配置", err)
		} else {
			selectedSkills = secondPhaseSkills
			log.Printf("[AICMDBSkillService] 阶段二 AI 选择的 Domain Skills: %v", selectedSkills)
		}
	}

	// 步骤 4: 组装 Prompt
	// 先记录步骤 3 的耗时
	if lastStepTimer != nil && len(completedSteps) < 3 {
		prevStepName := "CMDB查询+Skill选择"
		var stepSkills []string
		if isSecondPhase {
			prevStepName = "处理选择+Skill"
			stepSkills = selectedSkills // 第二步流程显示生成阶段选择的 Domain Skills
		} else {
			stepSkills = cmdbUsedSkills // 第一步流程显示 CMDB 查询阶段使用的 Skills
		}
		completedSteps = append(completedSteps, CompletedStep{
			Name:       prevStepName,
			ElapsedMs:  int64(lastStepTimer.ElapsedMs()),
			UsedSkills: stepSkills,
		})
	}

	reportProgress(4, "组装Skill", "正在组装 Skill 提示词...")

	return s.generateWithCMDBDataAndSkillsWithProgress(
		userID, moduleID, userDescription, workspaceID, organizationID,
		aiConfig, cmdbData, selectedSkills, currentConfig, mode, totalTimer,
		progressCallback, totalSteps, completedSteps, false, // skipStep4Display = false
	)
}

// generateWithCMDBDataAndSkillsWithProgress 使用 CMDB 数据和选中的 Skills 生成配置（带进度回调）
// skipStep4Display: 是否跳过步骤 4 的显示（第二步流程中，步骤 3 和步骤 4 合并显示）
// 【重要】此方法已移除 Schema 直接加载，AI 只使用 Module Skill 生成参数建议
// 然后由 SchemaSolver 进行验证和修正
func (s *AICMDBSkillService) generateWithCMDBDataAndSkillsWithProgress(
	userID string,
	moduleID uint,
	userDescription string,
	workspaceID string,
	organizationID string,
	aiConfig *models.AIConfig,
	cmdbData string,
	selectedSkills []string,
	currentConfig map[string]interface{},
	mode string,
	totalTimer *Timer,
	progressCallback ProgressCallback,
	totalSteps int,
	completedSteps []CompletedStep,
	skipStep4Display bool,
) (*GenerateConfigWithCMDBResponse, error) {
	// 辅助函数：安全地推送进度（带已完成步骤列表）
	reportProgress := func(step int, stepName, message string, steps []CompletedStep) {
		if progressCallback != nil {
			progressCallback(ProgressEvent{
				Type:           "progress",
				Step:           step,
				TotalSteps:     totalSteps,
				StepName:       stepName,
				Message:        message,
				CompletedSteps: steps,
			})
		}
	}

	// 获取 Skill 组合配置
	composition := &aiConfig.SkillComposition
	if len(composition.FoundationSkills) == 0 && composition.TaskSkill == "" {
		composition = s.getDefaultSkillComposition()
	}

	// 如果有 AI 选择的 Skills，覆盖 Domain Skills
	if len(selectedSkills) > 0 {
		composition.DomainSkills = selectedSkills
		composition.DomainSkillMode = models.DomainSkillModeFixed
	}

	// 【重要】移除 Schema 直接加载，AI 只使用 Module Skill
	// schemaData := s.getSchemaData(moduleID) // 已移除

	// 构建动态上下文（不再包含 SchemaData）
	dynamicContext := &DynamicContext{
		UserDescription: userDescription,
		WorkspaceID:     workspaceID,
		OrganizationID:  organizationID,
		ModuleID:        moduleID,
		UseCMDB:         cmdbData != "",
		CurrentConfig:   currentConfig,
		CMDBData:        cmdbData,
		SchemaData:      "", // 不再传递 Schema 数据给 AI
		ExtraContext: map[string]interface{}{
			"mode": mode,
		},
	}

	// 组装 Prompt
	assembleTimer := NewTimer()
	assembleResult, err := s.skillAssembler.AssemblePrompt(composition, moduleID, dynamicContext)
	if err != nil {
		IncAICallCount("form_generation_optimized", "skill_assembly_error")
		return nil, err
	}
	RecordSkillAssemblyDuration("form_generation_optimized", len(assembleResult.UsedSkillNames), assembleTimer.ElapsedMs())

	// 记录步骤 4（组装 Skill）的耗时
	// 如果 skipStep4Display 为 true，则不单独显示步骤 4，而是将时间合并到步骤 3
	if !skipStep4Display {
		completedSteps = append(completedSteps, CompletedStep{
			Name:       "组装Skill",
			ElapsedMs:  int64(assembleTimer.ElapsedMs()),
			UsedSkills: assembleResult.UsedSkillNames, // 添加使用的 Skills
		})
	} else {
		// 第二步流程中，将 Skills 信息添加到最后一个步骤（处理选择+Skill）
		if len(completedSteps) > 0 {
			completedSteps[len(completedSteps)-1].UsedSkills = assembleResult.UsedSkillNames
		}
	}

	// 步骤 5: AI 生成
	reportProgress(5, "AI生成", "正在调用 AI 生成配置...", completedSteps)
	aiTimer := NewTimer()
	aiResult, err := s.aiFormService.callAI(aiConfig, assembleResult.Prompt)
	RecordAICallDuration("form_generation_optimized", "ai_call", aiTimer.ElapsedMs())
	if err != nil {
		IncAICallCount("form_generation_optimized", "ai_error")
		return nil, err
	}

	// 记录 AI 生成步骤的耗时
	completedSteps = append(completedSteps, CompletedStep{
		Name:       "AI生成",
		ElapsedMs:  int64(aiTimer.ElapsedMs()),
		UsedSkills: nil, // AI 生成步骤不需要显示 Skills
	})

	// 解析 AI 响应
	response, err := s.parseAIResponse(aiResult, moduleID)
	if err != nil {
		IncAICallCount("form_generation_optimized", "parse_error")
		return nil, err
	}

	// 【新增】使用 SchemaSolver 验证 AI 生成的配置
	if response.Config != nil && len(response.Config) > 0 {
		solverTimer := NewTimer()
		solver := NewSchemaSolver(s.db, moduleID)
		solverResult := solver.Solve(response.Config)

		if !solverResult.Success {
			log.Printf("[AICMDBSkillService] SchemaSolver 验证失败，需要 AI 修正")
			log.Printf("[AICMDBSkillService] 反馈: %s", solverResult.AIInstructions)

			// 如果需要 AI 修正，使用反馈循环
			if solverResult.NeedAIFix {
				// 更新进度
				reportProgress(5, "Schema验证", "正在验证并修正配置...", completedSteps)

				// 使用反馈循环让 AI 修正
				loop := NewAIFeedbackLoop(s.db, moduleID)
				loop.SetMaxRetries(config.GetSchemaSolverMaxRetries())
				loopResult, loopErr := loop.ExecuteWithRetry(userDescription, response.Config, aiConfig)

				if loopErr != nil {
					log.Printf("[AICMDBSkillService] AI 反馈循环失败: %v", loopErr)
				} else if loopResult.Success {
					// 使用修正后的参数
					response.Config = loopResult.FinalParams
					log.Printf("[AICMDBSkillService] AI 修正成功，重试次数: %d", loopResult.TotalRetries)
				} else {
					// 修正失败，但仍然使用最终参数（可能部分正确）
					response.Config = loopResult.FinalParams
					log.Printf("[AICMDBSkillService] AI 修正未完全成功: %s", loopResult.Error)
					// 添加警告信息
					if response.Message == "" {
						response.Message = "配置已生成，但部分参数可能需要手动调整"
					}
				}

				// 记录 Schema 验证步骤的耗时
				completedSteps = append(completedSteps, CompletedStep{
					Name:      "Schema验证",
					ElapsedMs: int64(solverTimer.ElapsedMs()),
				})
			}
		} else {
			// 验证成功，使用 SchemaSolver 处理后的参数
			response.Config = solverResult.Params
			log.Printf("[AICMDBSkillService] SchemaSolver 验证成功: AI 提供的参数格式正确（枚举值、类型、依赖关系均通过验证）")
			if len(solverResult.AppliedRules) > 0 {
				log.Printf("[AICMDBSkillService] 触发了 %d 条规则: %v", len(solverResult.AppliedRules), solverResult.AppliedRules)
			} else {
				log.Printf("[AICMDBSkillService] 未触发任何隐含/条件规则，直接使用 AI 的输出")
			}
		}
	}

	// 发送最终进度（包含所有已完成步骤）
	reportProgress(5, "完成", "配置生成完成", completedSteps)

	// 记录指标
	executionTimeMs := int(totalTimer.ElapsedMs())
	RecordAICallDuration("form_generation_optimized", "total", totalTimer.ElapsedMs())
	IncAICallCount("form_generation_optimized", "success")

	if err := s.skillAssembler.LogSkillUsage(
		assembleResult.UsedSkillIDs,
		"form_generation",
		workspaceID,
		userID,
		&moduleID,
		aiConfig.ModelID,
		executionTimeMs,
	); err != nil {
		log.Printf("[AICMDBSkillService] 记录 Skill 使用日志失败: %v", err)
	}

	log.Printf("[AICMDBSkillService] ========== 优化版配置生成完成 ==========")
	return response, nil
}
