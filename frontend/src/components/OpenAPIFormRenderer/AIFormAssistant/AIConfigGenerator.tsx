import React, { useState, useCallback } from 'react';
import { Input, Button, message, Alert, Modal, Spin, Collapse, Tabs, Tooltip, Switch, Tag, Select, notification, Popover } from 'antd';
import { QuestionCircleOutlined, CloseOutlined, CheckCircleOutlined, WarningOutlined, EyeOutlined, CodeOutlined, ToolOutlined, CopyOutlined, CheckOutlined, DatabaseOutlined } from '@ant-design/icons';
import { generateFormConfig, generateFormConfigWithCMDB, generateFormConfigWithProgress } from '../../../services/aiForm';
import type { PlaceholderInfo, CMDBLookupResult, ProgressEvent, CompletedStep } from '../../../services/aiForm';
import styles from './AIConfigGenerator.module.css';

// 稳定的空数组常量，避免在组件 props 默认值中创建新引用导致无限循环
const EMPTY_CMDB_LOOKUPS: CMDBLookupResult[] = [];
const EMPTY_WARNINGS: string[] = [];

// 格式化步骤时间：时间很短的步骤（<500ms）固定显示为 0.5s
const formatStepTime = (elapsedMs: number): string => {
  if (elapsedMs < 500) {
    return '0.5';
  }
  return (elapsedMs / 1000).toFixed(1);
};

// AI 助手图标组件（蓝色笔+星星）
const AIAssistantIcon: React.FC<{ className?: string }> = ({ className }) => (
  <svg 
    className={className}
    width="16" 
    height="16" 
    viewBox="0 0 24 24" 
    fill="none" 
    xmlns="http://www.w3.org/2000/svg"
  >
    <path 
      d="M3 21V16.75L16.2 3.55C16.4 3.35 16.625 3.2 16.875 3.1C17.125 3 17.3833 2.95 17.65 2.95C17.9167 2.95 18.1792 3 18.4375 3.1C18.6958 3.2 18.9167 3.35 19.1 3.55L20.45 4.9C20.65 5.08333 20.8 5.30417 20.9 5.5625C21 5.82083 21.05 6.08333 21.05 6.35C21.05 6.61667 21 6.875 20.9 7.125C20.8 7.375 20.65 7.6 20.45 7.8L7.25 21H3ZM17.6 7.8L18.95 6.45L17.55 5.05L16.2 6.4L17.6 7.8Z" 
      fill="currentColor"
    />
    <path 
      d="M19 2L19.5 3.5L21 4L19.5 4.5L19 6L18.5 4.5L17 4L18.5 3.5L19 2Z" 
      fill="currentColor"
    />
    <path 
      d="M11 3L11.3 3.9L12.2 4.2L11.3 4.5L11 5.4L10.7 4.5L9.8 4.2L10.7 3.9L11 3Z" 
      fill="currentColor"
      opacity="0.6"
    />
  </svg>
);

type GenerateMode = 'new' | 'refine';

// 预设的建议语句（Tab 键自动填入）
const SUGGESTION_EXAMPLE = {
  cmdb: '在exchange VPC的东京1a创建一台ec2.安全组使用java-private ,主机名称名称使用abcd.使用t3.medium类型',
  normal: '创建一个生产环境的配置，启用高可用和加密，使用 t3.medium 实例类型',
};

// AI 提示词类型
export interface AIPromptItem {
  id: string;
  title: string;
  prompt: string;
  created_at: string;
}

// 进度信息类型
export interface ProgressInfo {
  step: number;
  totalSteps: number;
  stepName: string;
  message?: string;
  elapsedMs: number;
  completedSteps?: Array<{ name: string; elapsed_ms: number; used_skills?: string[] }>;
}

// 贯穿式输入面板组件 - 独立导出供父组件使用
export interface AIInputPanelProps {
  description: string;
  onDescriptionChange: (value: string) => void;
  onGenerate: (mode: GenerateMode) => void;
  onClose: () => void;
  loading: boolean;
  generateMode: GenerateMode;
  hasCurrentData: boolean;
  hasGeneratedConfig?: boolean;
  onPreview?: () => void;
  // CMDB 模式
  cmdbMode?: boolean;
  onCmdbModeChange?: (enabled: boolean) => void;
  // 提示词列表
  prompts?: AIPromptItem[];
  // 真实进度（从 SSE 获取）
  progress?: ProgressInfo | null;
  // 最终进度（用于显示执行摘要）
  finalProgress?: ProgressInfo | null;
}

// 进度步骤定义
const PROGRESS_STEPS = {
  cmdb: [
    { key: 'parse', label: '解析需求', duration: 800 },
    { key: 'cmdb', label: '查询CMDB', duration: 1500 },
    { key: 'skill', label: '组装Skill', duration: 600 },
    { key: 'ai', label: 'AI生成', duration: 0 },  // 最后一步持续到完成
  ],
  normal: [
    { key: 'parse', label: '解析需求', duration: 800 },
    { key: 'ai', label: 'AI生成', duration: 0 },  // 最后一步持续到完成
  ],
};

export const AIInputPanel: React.FC<AIInputPanelProps> = ({
  description,
  onDescriptionChange,
  onGenerate,
  onClose,
  loading,
  generateMode,
  hasCurrentData,
  hasGeneratedConfig = false,
  onPreview,
  cmdbMode = false,
  onCmdbModeChange,
  prompts = [],
  progress,  // 真实进度（从 SSE 获取）
  finalProgress,  // 最终进度（用于显示执行摘要）
}) => {
  // 提示词轮播状态
  const [currentScrollIndex, setCurrentScrollIndex] = React.useState(0);
  
  // 实时计时器（用于显示当前步骤的动态耗时）
  const [elapsedSeconds, setElapsedSeconds] = React.useState(0);
  const timerRef = React.useRef<ReturnType<typeof setInterval> | null>(null);
  
  // 自动轮播逻辑（每5秒切换一次，仅当有多个提示词时）
  React.useEffect(() => {
    if (prompts.length <= 1) return;
    
    const interval = setInterval(() => {
      setCurrentScrollIndex(prev => (prev + 1) % prompts.length);
    }, 5000);
    
    return () => clearInterval(interval);
  }, [prompts.length]);
  
  // 实时计时器逻辑
  // 每个步骤的时间都是独立的，从 0 开始计时
  React.useEffect(() => {
    if (loading && progress) {
      // 每次步骤变化时，重置计时器为 0
      setElapsedSeconds(0);
      
      // 清除之前的计时器
      if (timerRef.current) {
        clearInterval(timerRef.current);
      }
      
      // 开始新的计时
      timerRef.current = setInterval(() => {
        setElapsedSeconds(prev => prev + 1);
      }, 1000);
    } else {
      // 停止计时
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
      setElapsedSeconds(0);
    }
    
    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [loading, progress?.step]);
  
  // 处理键盘事件
  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // Tab 键：如果输入框为空，填入示例语句
    if (e.key === 'Tab' && !description.trim()) {
      e.preventDefault();
      const example = cmdbMode ? SUGGESTION_EXAMPLE.cmdb : SUGGESTION_EXAMPLE.normal;
      onDescriptionChange(example);
    }
    // Enter + Ctrl/Cmd：生成配置
    else if (e.key === 'Enter' && (e.ctrlKey || e.metaKey) && !loading) {
      onGenerate('new');
    }
  }, [description, cmdbMode, onDescriptionChange, loading, onGenerate]);
  
  return (
    <div className={styles.fullWidthPanel}>
      <div className={styles.panelHeader}>
        <span className={styles.promptLabel}>Prompt</span>
        <span className={styles.infoLink}>Info</span>
        {/* 提示词显示区域 - 放在 Info 旁边，多个提示词轮播显示 */}
        {prompts.length > 0 && (
          <span 
            className={styles.promptHint}
            onClick={() => onDescriptionChange(prompts[currentScrollIndex % prompts.length].prompt)}
            title={prompts[currentScrollIndex % prompts.length].title 
              ? `${prompts[currentScrollIndex % prompts.length].title}: ${prompts[currentScrollIndex % prompts.length].prompt}` 
              : prompts[currentScrollIndex % prompts.length].prompt}
          >
            {prompts.length > 1 && <span style={{ color: '#bbb', marginRight: 4 }}>[{(currentScrollIndex % prompts.length) + 1}/{prompts.length}]</span>}
            {prompts[currentScrollIndex % prompts.length].prompt}
          </span>
        )}
        <span className={styles.charCount}>{description.length}/500</span>
        <button className={styles.panelCloseButton} onClick={onClose}>
          <CloseOutlined />
        </button>
      </div>
      
      <div className={styles.inputSection}>
        <Input.TextArea
          value={description}
          onChange={(e) => onDescriptionChange(e.target.value)}
          placeholder={cmdbMode 
            ? '按 Tab 键填入示例，或输入: "在 exchange vpc 的东京1a区域创建 ec2"'
            : '按 Tab 键填入示例，或输入: "创建一个生产环境的配置"'
          }
          maxLength={500}
          autoSize={{ minRows: 2, maxRows: 10 }}
          className={styles.fullWidthTextArea}
          disabled={loading}
          autoFocus
          onKeyDown={handleKeyDown}
        />
      </div>
      
      <div className={styles.actionBar}>
        <div className={styles.actionButtons}>
          <Button
            onClick={() => onGenerate('new')}
            loading={loading && generateMode === 'new'}
            disabled={!description.trim() || loading}
            className={styles.actionButton}
            icon={cmdbMode ? <DatabaseOutlined /> : undefined}
          >
            {cmdbMode ? '智能生成' : '生成新配置'}
          </Button>
          <Button
            onClick={() => onGenerate('refine')}
            loading={loading && generateMode === 'refine'}
            disabled={loading || !hasCurrentData}
            className={styles.actionButton}
            icon={<ToolOutlined />}
            title={hasCurrentData ? '基于当前表单数据进行修复和优化（无需输入描述）' : '请先填写一些表单数据'}
          >
            修复现有配置
          </Button>
          {hasGeneratedConfig && onPreview && (
            <Button
              onClick={onPreview}
              className={styles.actionButton}
              icon={<EyeOutlined />}
              type="dashed"
            >
              预览生成数据
            </Button>
          )}
        </div>
        {/* 进度显示 - 独立一行显示，支持换行 */}
        {/* 加载中：显示实时进度 */}
        {/* 加载完成：显示执行摘要 */}
        {loading ? (
          <div style={{ width: '100%', display: 'flex', alignItems: 'flex-start', gap: 8, marginTop: 8, flexWrap: 'wrap' }}>
            <Spin size="small" />
            <div style={{ flex: 1, minWidth: 0 }}>
              {progress ? (
                // 使用真实进度数据
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, alignItems: 'center', fontSize: 12 }}>
                  {/* 已完成的步骤（绿色） */}
                  {progress.completedSteps?.map((step, index) => (
                    <React.Fragment key={index}>
                      <span style={{ color: '#52c41a', whiteSpace: 'nowrap' }}>
                        ✓{step.name}({(step.elapsed_ms / 1000).toFixed(1)}s)
                      </span>
                      <span style={{ color: '#d9d9d9' }}>→</span>
                    </React.Fragment>
                  ))}
                  {/* 当前步骤（蓝色，带动态计时） */}
                  <span style={{ color: '#4F7CFF', fontWeight: 500, whiteSpace: 'nowrap' }}>
                    {progress.stepName}({elapsedSeconds}s)
                  </span>
                </div>
              ) : (
                // 没有进度数据时显示默认文本
                <span style={{ fontSize: 12, color: '#999' }}>正在处理...</span>
              )}
            </div>
          </div>
        ) : finalProgress?.completedSteps && finalProgress.completedSteps.length > 0 ? (
          // 执行摘要 - 独立一行显示
          <div style={{ width: '100%', marginTop: 8, padding: '8px 0', borderTop: '1px dashed #e8e8e8' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 8 }}>
              <CheckCircleOutlined style={{ color: '#52c41a', marginTop: 2 }} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, alignItems: 'center', fontSize: 12 }}>
                  {finalProgress.completedSteps.map((step, index) => {
                    // 检查是否是两步流程的分界点（第一步的最后一个步骤后面是第二步的初始化）
                    const nextStep = finalProgress.completedSteps![index + 1];
                    const isPhaseBreak = nextStep?.name === '初始化' && index > 0;
                    
                    // 检查当前步骤是否在第二步流程中（在"用户选择"分界点之后）
                    // 找到分界点的位置
                    const phaseBreakIndex = finalProgress.completedSteps!.findIndex((s, i) => {
                      const next = finalProgress.completedSteps![i + 1];
                      return next?.name === '初始化' && i > 0;
                    });
                    const isInSecondPhase = phaseBreakIndex >= 0 && index > phaseBreakIndex;
                    
                    // 只有在第一步流程中，非初始化步骤如果时间小于 100ms 才显示"跳过"
                    // 第二步流程中的步骤都是真正执行的，不应该显示"跳过"
                    // 如果步骤有 used_skills，说明确实执行了，不应该显示"跳过"
                    const hasUsedSkills = step.used_skills && step.used_skills.length > 0;
                    const isSkipped = !isInSecondPhase && step.name !== '初始化' && step.elapsed_ms < 100 && !hasUsedSkills;
                    
                    // 构建 Tooltip 内容（显示使用的 Skills）
                    const skillsTooltip = step.used_skills && step.used_skills.length > 0 
                      ? (
                        <div style={{ fontSize: 11 }}>
                          <div style={{ fontWeight: 500, marginBottom: 4 }}>使用的 Skills:</div>
                          {step.used_skills.map((skill, i) => (
                            <div key={i} style={{ color: '#52c41a' }}>• {skill}</div>
                          ))}
                        </div>
                      )
                      : null;
                    
                    const stepContent = isSkipped 
                      ? `○${step.name}(跳过)` 
                      : `✓${step.name}(${formatStepTime(step.elapsed_ms)}s)`;
                    
                    return (
                      <React.Fragment key={index}>
                        {skillsTooltip ? (
                          <Tooltip title={skillsTooltip} placement="top">
                            <span 
                              style={{ 
                                color: isSkipped ? '#999' : '#52c41a', 
                                whiteSpace: 'nowrap',
                                cursor: 'help', 
                                borderBottom: '1px dashed currentColor' 
                              }}
                            >
                              {stepContent}
                            </span>
                          </Tooltip>
                        ) : (
                          <span style={{ color: isSkipped ? '#999' : '#52c41a', whiteSpace: 'nowrap' }}>
                            {stepContent}
                          </span>
                        )}
                        {index < finalProgress.completedSteps!.length - 1 && (
                          isPhaseBreak ? (
                            // 两步流程的分界点，显示"用户选择"
                            <>
                              <span style={{ color: '#d9d9d9' }}>→</span>
                              <span style={{ color: '#333', fontWeight: 500, whiteSpace: 'nowrap' }}>用户选择</span>
                              <span style={{ color: '#d9d9d9' }}>→</span>
                            </>
                          ) : (
                            <span style={{ color: '#d9d9d9' }}>→</span>
                          )
                        )}
                      </React.Fragment>
                    );
                  })}
                  {/* 总耗时跟在步骤后面 */}
                  <span style={{ color: '#666', marginLeft: 4, whiteSpace: 'nowrap' }}>
                    | 总耗时: {formatStepTime(finalProgress.completedSteps.reduce((sum, s) => sum + s.elapsed_ms, 0))}s
                  </span>
                </div>
              </div>
            </div>
          </div>
        ) : null}
      </div>
      
      {/* CMDB 开关和帮助文本 */}
      <div className={styles.helpText}>
        {onCmdbModeChange && (
          <Tooltip title="启用后，AI 会自动从 CMDB 查询您描述中提到的资源（如 VPC、子网、安全组等）">
            <div style={{ display: 'inline-flex', alignItems: 'center', gap: 4, marginRight: 12 }}>
              <Switch 
                size="small" 
                checked={cmdbMode} 
                onChange={onCmdbModeChange}
                disabled={loading}
              />
              <span style={{ fontSize: 12, color: cmdbMode ? '#1890ff' : '#666' }}>
                <DatabaseOutlined style={{ marginRight: 4 }} />
                CMDB 智能查询
              </span>
            </div>
          </Tooltip>
        )}
        <span style={{ color: '#999' }}>
          {hasCurrentData 
            ? '当前表单已有数据，可以选择"修复现有配置"进行优化'
            : '输入描述后点击"生成新配置"'
          }
        </span>
      </div>
      
    </div>
  );
};

// Hook: 用于管理 AI 助手状态
export interface UseAIConfigGeneratorOptions {
  moduleId: number;
  workspaceId?: string;
  organizationId?: string;
  manifestId?: string;
  currentFormData?: Record<string, unknown>;
  onGenerate: (config: Record<string, unknown>) => void;
}

// 空值字段信息
export interface EmptyFieldInfo {
  field: string;
  description: string;
  type: 'empty_in_ai' | 'missing_in_ai' | 'user_empty';
}

// 检测配置中的占位符（前端检测，用于合并后的配置）
// 支持多种占位符格式：
// - <YOUR_XXX> 格式（AI 生成的标准格式）
// - <XXX> 格式（简化格式）
// - {{XXX}} 格式（模板格式）
// - ${XXX} 格式（变量格式）
const detectPlaceholdersInConfig = (config: Record<string, unknown>): PlaceholderInfo[] => {
  const placeholders: PlaceholderInfo[] = [];
  // 更通用的正则：匹配 <YOUR_...>、<...>、{{...}}、${...} 等格式
  // 支持字母、数字、下划线、连字符
  const placeholderPattern = /<YOUR_[A-Za-z0-9_-]+>|<[A-Z][A-Za-z0-9_-]*>|\{\{[A-Za-z0-9_-]+\}\}|\$\{[A-Za-z0-9_-]+\}/g;
  
  const scan = (obj: unknown, path: string) => {
    if (typeof obj === 'string') {
      const matches = obj.match(placeholderPattern);
      if (matches) {
        matches.forEach(match => {
          // 避免重复添加
          if (!placeholders.some(p => p.field === path && p.placeholder === match)) {
            placeholders.push({
              field: path,
              placeholder: match,
              description: `请替换 ${match} 为实际值`,
            });
          }
        });
      }
    } else if (Array.isArray(obj)) {
      obj.forEach((item, i) => scan(item, `${path}[${i}]`));
    } else if (obj && typeof obj === 'object') {
      Object.entries(obj as Record<string, unknown>).forEach(([key, value]) => {
        const newPath = path ? `${path}.${key}` : key;
        scan(value, newPath);
      });
    }
  };
  
  scan(config, '');
  return placeholders;
};

// 检测配置中的空字符串值
// 注意：这个函数现在不再使用，因为 AI 生成的空字符串通常是有意义的占位符
// 我们只检测用户添加但 AI 没有填充的空值字段
const detectEmptyFields = (_config: Record<string, unknown>, _path: string = ''): EmptyFieldInfo[] => {
  // 不再检测 AI 生成配置中的空字符串值
  // 因为这些空字符串可能是 AI 故意生成的，表示"这个字段存在但需要用户填写"
  // 用户可以在预览中看到这些字段，并决定是否填写
  return [];
};

// 检测用户数据中存在但 AI 没有填充的空值字段
const detectUserEmptyFields = (
  userData: Record<string, unknown>, 
  aiData: Record<string, unknown>,
  path: string = ''
): EmptyFieldInfo[] => {
  const emptyFields: EmptyFieldInfo[] = [];
  
  const scan = (userObj: unknown, aiObj: unknown, currentPath: string) => {
    if (typeof userObj === 'object' && userObj !== null && !Array.isArray(userObj)) {
      const userRecord = userObj as Record<string, unknown>;
      const aiRecord = (aiObj && typeof aiObj === 'object' && !Array.isArray(aiObj)) 
        ? aiObj as Record<string, unknown> 
        : {};
      
      Object.entries(userRecord).forEach(([key, userValue]) => {
        const newPath = currentPath ? `${currentPath}.${key}` : key;
        const aiValue = aiRecord[key];
        
        // 用户添加了 key 但 value 为空，且 AI 也没有提供值
        if ((userValue === '' || userValue === null || userValue === undefined) && 
            (aiValue === '' || aiValue === null || aiValue === undefined)) {
          emptyFields.push({
            field: newPath,
            description: '您添加了此字段但未填写值，AI 也无法确定应填写什么值，请手动填写',
            type: 'user_empty',
          });
        }
        // 递归检查嵌套对象
        else if (typeof userValue === 'object' && userValue !== null && !Array.isArray(userValue)) {
          scan(userValue, aiValue, newPath);
        }
      });
    }
  };
  
  scan(userData, aiData, path);
  return emptyFields;
};

export interface UseAIConfigGeneratorReturn {
  // 状态
  expanded: boolean;
  description: string;
  loading: boolean;
  generateMode: GenerateMode;
  hasCurrentData: boolean;
  previewOpen: boolean;
  generatedConfig: Record<string, unknown> | null;
  mergedConfig: Record<string, unknown> | null;  // 合并后的完整配置（用于预览）
  placeholders: PlaceholderInfo[];
  emptyFields: EmptyFieldInfo[];
  hasGeneratedConfig: boolean;
  blockMessage: string | null;  // 拦截消息
  // CMDB 模式
  cmdbMode: boolean;
  cmdbLookups: CMDBLookupResult[];
  warnings: string[];
  needSelection: boolean;
  // 进度（从 SSE 获取）
  progress: ProgressInfo | null;
  finalProgress: ProgressInfo | null;
  // 操作
  setExpanded: (expanded: boolean) => void;
  setDescription: (description: string) => void;
  setCmdbMode: (enabled: boolean) => void;
  handleGenerate: (mode: GenerateMode) => Promise<void>;
  handleGenerateWithSelections: (userSelections: Record<string, string | string[]>) => Promise<void>;  // 带用户选择的重新生成
  handleApplyConfig: () => void;
  setPreviewOpen: (open: boolean) => void;
  openPreview: () => void;
  // 渲染辅助
  renderConfigValue: (value: unknown) => React.ReactNode;
}

// 过滤掉对象中的空字符串值
const filterEmptyStrings = (obj: Record<string, unknown>): Record<string, unknown> => {
  const result: Record<string, unknown> = {};
  
  Object.keys(obj).forEach(key => {
    const value = obj[key];
    
    // 跳过空字符串
    if (value === '') {
      return;
    }
    
    // 递归处理嵌套对象
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const filtered = filterEmptyStrings(value as Record<string, unknown>);
      if (Object.keys(filtered).length > 0) {
        result[key] = filtered;
      }
    } else {
      result[key] = value;
    }
  });
  
  return result;
};

// 应用用户选择的资源 ID 到配置中
// 这个函数会查找配置中与资源类型相关的字段，并替换为用户选中的 ID
const applyResourceSelections = (
  config: Record<string, unknown>, 
  resourceIdMap: Record<string, string | string[]>
): Record<string, unknown> => {
  const result = { ...config };
  
  // 遍历资源 ID 映射
  Object.entries(resourceIdMap).forEach(([targetField, selectedIds]) => {
    // 尝试找到配置中对应的字段
    // 字段名可能是：security_group_ids, subnet_id, vpc_id, security_group_ids_ids 等
    const possibleFieldNames = [
      targetField,                           // 原始字段名，如 security_group_ids
      targetField.replace(/_ids$/, '_id'),   // 单数形式，如 security_group_id
      targetField.replace(/_id$/, '_ids'),   // 复数形式
    ];
    
    // 查找并替换
    for (const fieldName of possibleFieldNames) {
      if (fieldName in result) {
        const currentValue = result[fieldName];
        
        if (Array.isArray(selectedIds)) {
          // 多选情况：直接使用选中的 ID 数组
          result[fieldName] = selectedIds;
        } else {
          // 单选情况
          if (Array.isArray(currentValue)) {
            // 如果原来是数组，替换为包含单个 ID 的数组
            result[fieldName] = [selectedIds];
          } else {
            // 如果原来是字符串，直接替换
            result[fieldName] = selectedIds;
          }
        }
        break;  // 找到并替换后退出循环
      }
    }
    
    // 如果没有找到对应字段，尝试在嵌套对象中查找
    // 这里简化处理，只处理顶层字段
  });
  
  return result;
};

// 智能合并函数：AI 数据优先，用户数据作为补充
const smartMergeConfig = (userData: Record<string, unknown>, aiData: Record<string, unknown>): Record<string, unknown> => {
  // 以用户数据为基础（保留用户手动添加的字段）
  const result = { ...userData };
  
  // 遍历 AI 生成的数据，AI 的值优先
  Object.keys(aiData).forEach(key => {
    const aiValue = aiData[key];
    const userValue = result[key];
    
    // 过滤掉 AI 生成的空字符串值（AI 不应该生成空字符串）
    if (aiValue === '') {
      return;
    }
    
    // 如果 AI 的值是对象，需要特殊处理
    if (aiValue && typeof aiValue === 'object' && !Array.isArray(aiValue)) {
      // 过滤掉对象中的空字符串
      const filteredAiValue = filterEmptyStrings(aiValue as Record<string, unknown>);
      
      // 如果过滤后的对象为空，跳过
      if (Object.keys(filteredAiValue).length === 0) {
        return;
      }
      
      // 如果用户数据中也有这个字段且是对象，递归合并
      if (userValue && typeof userValue === 'object' && !Array.isArray(userValue)) {
        result[key] = smartMergeConfig(userValue as Record<string, unknown>, filteredAiValue);
      } else {
        // 否则直接使用 AI 的值
        result[key] = filteredAiValue;
      }
      return;
    }
    
    // 对于非对象值，AI 的值直接覆盖用户数据
    result[key] = aiValue;
  });
  
  return result;
};

export const useAIConfigGenerator = (options: UseAIConfigGeneratorOptions): UseAIConfigGeneratorReturn => {
  const { moduleId, workspaceId, organizationId, manifestId, currentFormData, onGenerate } = options;
  
  const [expanded, setExpanded] = useState(false);
  const [description, setDescription] = useState('');
  const [loading, setLoading] = useState(false);
  const [generateMode, setGenerateMode] = useState<GenerateMode>('new');
  const [previewOpen, setPreviewOpen] = useState(false);
  const [generatedConfig, setGeneratedConfig] = useState<Record<string, unknown> | null>(null);
  const [mergedConfig, setMergedConfig] = useState<Record<string, unknown> | null>(null);  // 合并后的完整配置
  const [placeholders, setPlaceholders] = useState<PlaceholderInfo[]>([]);
  const [emptyFields, setEmptyFields] = useState<EmptyFieldInfo[]>([]);
  const [currentMode, setCurrentMode] = useState<GenerateMode>('new');  // 记录当前生成模式
  const [blockMessage, setBlockMessage] = useState<string | null>(null);  // 拦截消息
  
  // 进度状态（用于实时显示和执行摘要）
  const [progress, setProgress] = useState<ProgressInfo | null>(null);
  const progressRef = React.useRef<ProgressInfo | null>(null);
  // 单独保存最后一个包含 completedSteps 的进度（用于执行摘要）
  const lastCompletedStepsRef = React.useRef<Array<{ name: string; elapsed_ms: number }> | null>(null);
  // 保存第一步（need_selection 之前）的进度，用于累加
  const firstPhaseStepsRef = React.useRef<Array<{ name: string; elapsed_ms: number }> | null>(null);
  const [finalProgress, setFinalProgress] = useState<ProgressInfo | null>(null);
  
  // CMDB 模式状态 - 默认打开
  const [cmdbMode, setCmdbMode] = useState(true);
  const [cmdbLookups, setCmdbLookups] = useState<CMDBLookupResult[]>([]);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [needSelection, setNeedSelection] = useState(false);

  // 检查是否有当前数据 - 更宽松的判断，只要有任何非空值就认为有数据
  const hasCurrentData = !!(currentFormData && Object.keys(currentFormData).some(key => {
    const value = currentFormData[key];
    if (value === null || value === undefined || value === '') return false;
    if (Array.isArray(value) && value.length === 0) return false;
    if (typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0) return false;
    return true;
  }));

  // 带用户选择的重新生成（用于 CMDB 多选后）
  // 方案：先调用 AI 生成配置，然后在前端直接替换用户选中的资源 ID
  const handleGenerateWithSelections = useCallback(async (userSelections: Record<string, string | string[]>) => {
    if (!description.trim()) {
      message.warning('请输入配置描述');
      return;
    }

    setLoading(true);
    setGenerateMode('new');
    setCurrentMode('new');
    
    // 不清除 CMDB 状态，保留之前的查询结果
    setNeedSelection(false);
    
    // 保存第一步的进度数据（need_selection 之前的步骤）
    // 这样最终可以累加两步的耗时
    if (lastCompletedStepsRef.current && lastCompletedStepsRef.current.length > 0) {
      firstPhaseStepsRef.current = [...lastCompletedStepsRef.current];
    }
    
    // 清除之前的进度状态，避免显示两个执行摘要
    setFinalProgress(null);
    lastCompletedStepsRef.current = null;

    try {
      // 构建增强的描述：原始描述 + CMDB 查询到的资源 ID
      let enhancedDescription = description;
      
      // 构建资源信息映射（包含完整的 ARN）
      // 格式：{ targetField: { id, name, arn } } 或 { targetField: [{ id, name, arn }, ...] }
      const resourceInfoMap: Record<string, any> = {};
      
      // 从 cmdbLookups 中获取资源信息，结合用户选择
      const resourceInfo: string[] = [];
      cmdbLookups.forEach(lookup => {
        const targetField = (lookup as any).target_field || lookup.resource_type;
        const selectedId = userSelections[targetField];
        
        if (selectedId) {
          // 用户选择了特定资源，从 candidates 中找到完整信息
          if (Array.isArray(selectedId)) {
            // 多选情况：找到所有选中的资源的完整信息
            const selectedResources = selectedId.map(id => {
              const candidate = lookup.candidates?.find(c => c.id === id);
              return candidate || { id, name: id, arn: '' };
            });
            resourceInfoMap[targetField] = selectedResources;
            // 使用 ARN（如果有）或 ID
            const arns = selectedResources.map(r => r.arn || r.id);
            resourceInfo.push(`${targetField} = ${JSON.stringify(arns)}`);
          } else {
            // 单选情况：找到选中资源的完整信息
            const candidate = lookup.candidates?.find(c => c.id === selectedId);
            const resourceData = candidate || lookup.result || { id: selectedId, name: selectedId, arn: '' };
            resourceInfoMap[targetField] = resourceData;
            // 使用 ARN（如果有）或 ID
            const arnOrId = resourceData.arn || resourceData.id;
            resourceInfo.push(`${targetField} = "${arnOrId}"`);
          }
        } else if (lookup.result) {
          // 使用唯一结果
          resourceInfoMap[targetField] = lookup.result;
          const arnOrId = lookup.result.arn || lookup.result.id;
          resourceInfo.push(`${targetField} = "${arnOrId}"`);
        }
      });
      
      // 同时构建简单的 ID 映射（用于后端兼容）
      const resourceIdMap: Record<string, string | string[]> = {};
      Object.entries(resourceInfoMap).forEach(([key, value]) => {
        if (Array.isArray(value)) {
          resourceIdMap[key] = value.map((r: any) => r.id);
        } else {
          resourceIdMap[key] = value.id;
        }
      });
      
      if (resourceInfo.length > 0) {
        enhancedDescription += '\n\n【重要：请使用以下 CMDB 查询到的资源 ID 填充配置】\n';
        enhancedDescription += resourceInfo.join('\n');
        enhancedDescription += '\n\n请将上述资源 ID 直接填入对应的配置字段中，不要使用占位符。';
      }
      
      // 调用 CMDB Skill API（使用 SSE 实时进度），传递完整的资源信息（包括 ARN）
      // 这样后端不需要再次查询数据库
      const response = await generateFormConfigWithProgress(
        {
          moduleId,
          description,  // 使用原始描述，不需要增强
          contextIds: {
            workspace_id: workspaceId,
            organization_id: organizationId,
          },
          mode: 'new',
          userSelections: resourceIdMap as Record<string, string>,  // 传递用户选择的 ID
          useOptimized: false,
          resourceInfoMap,  // 传递完整的资源信息（包括 ARN）
        },
        // 进度回调
        (event: ProgressEvent) => {
          const newProgress: ProgressInfo = {
            step: event.step,
            totalSteps: event.total_steps,
            stepName: event.step_name,
            message: event.message,
            elapsedMs: event.elapsed_ms,
            completedSteps: event.completed_steps,
          };
          progressRef.current = newProgress;  // 保存到 ref
          // 只有当事件包含 completedSteps 时才更新 lastCompletedStepsRef
          if (event.completed_steps && event.completed_steps.length > 0) {
            lastCompletedStepsRef.current = event.completed_steps;
          }
          setProgress(newProgress);  // 触发重新渲染
        }
      );
      
      // 保存最终进度（使用 lastCompletedStepsRef 确保 completedSteps 不会被 need_selection 事件覆盖）
      // 合并第一步和第二步的进度
      const secondPhaseSteps = lastCompletedStepsRef.current ?? progressRef.current?.completedSteps ?? [];
      const allCompletedSteps = [
        ...(firstPhaseStepsRef.current ?? []),
        ...(secondPhaseSteps ?? []),
      ];
      
      const finalProgressData: ProgressInfo = {
        step: progressRef.current?.step ?? 0,
        totalSteps: progressRef.current?.totalSteps ?? 0,
        stepName: progressRef.current?.stepName ?? '',
        message: progressRef.current?.message,
        elapsedMs: progressRef.current?.elapsedMs ?? 0,
        completedSteps: allCompletedSteps.length > 0 ? allCompletedSteps : undefined,
      };
      setFinalProgress(finalProgressData);
      setProgress(null);  // 清除实时进度
      
      // 清除第一步的进度数据
      firstPhaseStepsRef.current = null;

      // 处理被拦截的情况
      if (response.status === 'blocked') {
        setBlockMessage(response.message || '请求已被安全系统拦截');
        setGeneratedConfig(null);
        setMergedConfig(null);
        setPlaceholders([]);
        setEmptyFields([]);
        return;
      }

      // 清除之前的拦截消息
      setBlockMessage(null);

      let config = response.config || null;
      
      // 【关键】在前端直接替换用户选中的资源 ID
      // 这样就不依赖 AI 是否正确理解了
      if (config) {
        config = applyResourceSelections(config, resourceIdMap);
      }
      
      setGeneratedConfig(config);
      setMergedConfig(config);
      
      const mergedPlaceholders = config ? detectPlaceholdersInConfig(config) : [];
      setPlaceholders(mergedPlaceholders);
      
      const aiEmptyFields = config ? detectEmptyFields(config) : [];
      setEmptyFields(aiEmptyFields);

    } catch (error: unknown) {
      console.error('[AIConfigGenerator] Error:', error);
      message.error('生成配置失败');
    } finally {
      setLoading(false);
    }
  }, [description, moduleId, workspaceId, organizationId, manifestId, cmdbLookups]);

  const handleGenerate = useCallback(async (mode: GenerateMode) => {
    // 对于 refine 模式，如果没有描述，使用默认描述
    let effectiveDescription = description.trim();
    if (mode === 'refine' && !effectiveDescription && currentFormData) {
      // 构建简洁的默认描述（不包含完整配置，因为配置会通过 current_config 参数传递）
      const configKeys = Object.keys(currentFormData).filter(key => {
        const value = currentFormData[key];
        if (value === null || value === undefined || value === '') return false;
        if (Array.isArray(value) && value.length === 0) return false;
        if (typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0) return false;
        return true;
      });
      const fieldsSummary = configKeys.slice(0, 5).join(', ') + (configKeys.length > 5 ? '等' : '');
      effectiveDescription = `请分析并优化当前Terraform配置（包含${fieldsSummary}字段），检查是否完整、是否有遗漏或可改进的地方`;
    }
    
    if (!effectiveDescription) {
      message.warning('请输入配置描述');
      return;
    }

    setLoading(true);
    setGenerateMode(mode);
    setCurrentMode(mode);  // 记录当前模式
    
    // 清除之前的 CMDB 状态
    setCmdbLookups([]);
    setWarnings([]);
    setNeedSelection(false);

    try {
      let response;
      
      // 修复模式（refine）不需要查询 CMDB，直接调用普通 AI API
      // CMDB 模式只在新建模式（new）下生效
      if (cmdbMode && mode === 'new') {
        // CMDB 模式：调用带 SSE 实时进度的 API（仅新建模式）
        const cmdbResponse = await generateFormConfigWithProgress(
          {
            moduleId,
            description: effectiveDescription,
            contextIds: {
              workspace_id: workspaceId,
              organization_id: organizationId,
            },
            mode: 'new',
            useOptimized: false,
          },
        // 进度回调
        (event: ProgressEvent) => {
          const newProgress: ProgressInfo = {
            step: event.step,
            totalSteps: event.total_steps,
            stepName: event.step_name,
            message: event.message,
            elapsedMs: event.elapsed_ms,
            completedSteps: event.completed_steps,
          };
          progressRef.current = newProgress;  // 保存到 ref
          // 只有当事件包含 completedSteps 时才更新 lastCompletedStepsRef
          if (event.completed_steps && event.completed_steps.length > 0) {
            lastCompletedStepsRef.current = event.completed_steps;
          }
          setProgress(newProgress);  // 触发重新渲染
        }
      );
      
      // 保存最终进度（使用 lastCompletedStepsRef 确保 completedSteps 不会被 need_selection 事件覆盖）
      const finalProgressData: ProgressInfo = {
        step: progressRef.current?.step ?? 0,
        totalSteps: progressRef.current?.totalSteps ?? 0,
        stepName: progressRef.current?.stepName ?? '',
        message: progressRef.current?.message,
        elapsedMs: progressRef.current?.elapsedMs ?? 0,
        completedSteps: lastCompletedStepsRef.current ?? progressRef.current?.completedSteps,
      };
      setFinalProgress(finalProgressData);
      setProgress(null);  // 清除实时进度
      
      // 处理 CMDB 特有的状态
        if (cmdbResponse.cmdb_lookups) {
          setCmdbLookups(cmdbResponse.cmdb_lookups);
        }
        if (cmdbResponse.warnings) {
          setWarnings(cmdbResponse.warnings);
        }
        if (cmdbResponse.status === 'need_selection') {
          setNeedSelection(true);
          setBlockMessage(null);
          setGeneratedConfig(null);
          setMergedConfig(null);
          setPlaceholders([]);
          setEmptyFields([]);
          setPreviewOpen(true);
          return;
        }
        
        response = cmdbResponse;
      } else {
        // 普通模式或修复模式：调用原有 API
        response = await generateFormConfig(
          moduleId, 
          effectiveDescription, 
          {
            workspace_id: workspaceId,
            organization_id: organizationId,
            manifest_id: manifestId,
          },
          mode === 'refine' ? currentFormData : undefined,
          mode
        );
      }

      // 处理被拦截的情况
      if (response.status === 'blocked') {
        setBlockMessage(response.message || '请求已被安全系统拦截');
        setGeneratedConfig(null);
        setMergedConfig(null);
        setPlaceholders([]);
        setEmptyFields([]);
        setPreviewOpen(true);  // 打开预览弹窗显示拦截信息
        return;
      }

      // 清除之前的拦截消息
      setBlockMessage(null);

      const config = response.config || null;
      setGeneratedConfig(config);
      
      // 计算合并后的完整配置（用于预览）
      // 如果是修复模式，合并当前表单数据和 AI 生成的数据
      // 如果是新建模式，直接使用 AI 生成的数据
      let merged: Record<string, unknown> | null = null;
      if (config) {
        if (mode === 'refine' && currentFormData) {
          merged = smartMergeConfig(currentFormData, config);
        } else {
          merged = config;
        }
      }
      setMergedConfig(merged);
      
      // 基于合并后的配置检测占位符（而不是仅基于 AI 增量配置）
      // 这样可以正确检测用户数据中是否还有占位符
      const mergedPlaceholders = merged ? detectPlaceholdersInConfig(merged) : [];
      setPlaceholders(mergedPlaceholders);
      
      // 检测空字符串值
      let allEmptyFields: EmptyFieldInfo[] = [];
      
      if (config) {
        // 检测 AI 生成配置中的空字符串值
        const aiEmptyFields = detectEmptyFields(config);
        allEmptyFields = [...aiEmptyFields];
        
        // 如果是修复模式，还要检测用户添加但 AI 没有填充的空值字段
        if (mode === 'refine' && currentFormData) {
          const userEmptyFields = detectUserEmptyFields(currentFormData, config);
          allEmptyFields = [...allEmptyFields, ...userEmptyFields];
        }
      }
      
      setEmptyFields(allEmptyFields);
      setPreviewOpen(true);

    } catch (error: unknown) {
      console.error('[AIConfigGenerator] Error:', error);
      message.error('生成配置失败');
    } finally {
      setLoading(false);
    }
  }, [description, currentFormData, moduleId, workspaceId, organizationId, manifestId, cmdbMode]);

  const handleApplyConfig = useCallback(() => {
    // 在修复模式下，应用合并后的完整配置
    // 在新建模式下，应用 AI 生成的配置
    const configToApply = currentMode === 'refine' && mergedConfig ? mergedConfig : generatedConfig;
    if (configToApply) {
      onGenerate(configToApply);
      
      // 构建执行摘要消息
      // 优先使用 finalProgress 状态（在请求完成时保存的最终进度）
      // 如果 finalProgress 为 null，则使用 progressRef.current
      // 注意：progressRef.current 是一个 ref，不会触发重新渲染，但在回调中可以获取最新值
      const progressData = finalProgress ?? progressRef.current;
      
      let summaryMessage = '配置已应用到表单';
      if (progressData?.completedSteps && progressData.completedSteps.length > 0) {
        const stepsStr = progressData.completedSteps
          .map((s: { name: string; elapsed_ms: number }) => `✓${s.name}(${(s.elapsed_ms / 1000).toFixed(1)}s)`)
          .join(' → ');
        const totalMs = progressData.completedSteps.reduce((sum: number, s: { name: string; elapsed_ms: number }) => sum + s.elapsed_ms, 0);
        summaryMessage = `配置已应用 | ${stepsStr} | 总耗时: ${(totalMs / 1000).toFixed(1)}s`;
      }
      
      // 使用 antd notification 显示执行摘要（比 message 更可靠）
      // 构建带有"用户选择"标记和跳过标记的描述（使用 React 组件支持换行）
      let notificationDescription: React.ReactNode = '配置已成功应用到表单';
      if (progressData?.completedSteps && progressData.completedSteps.length > 0) {
        // 找到分界点的位置
        const phaseBreakIndex = progressData.completedSteps.findIndex((s: { name: string; elapsed_ms: number }, i: number) => {
          const next = progressData.completedSteps![i + 1];
          return next?.name === '初始化' && i > 0;
        });
        
        const totalMs = progressData.completedSteps.reduce((sum: number, s: { name: string; elapsed_ms: number }) => sum + s.elapsed_ms, 0);
        
        // 使用 React 组件构建描述，支持换行
        notificationDescription = (
          <div style={{ fontSize: 12 }}>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, alignItems: 'center' }}>
              {progressData.completedSteps.map((s: { name: string; elapsed_ms: number }, index: number) => {
                // 检查是否是两步流程的分界点（第一步的最后一个步骤后面是第二步的初始化）
                const nextStep = progressData.completedSteps![index + 1];
                const isPhaseBreak = nextStep?.name === '初始化' && index > 0;
                
                // 检查当前步骤是否在第二步流程中
                const isInSecondPhase = phaseBreakIndex >= 0 && index > phaseBreakIndex;
                
                // 只有在第一步流程中，非初始化步骤如果时间小于 100ms 才显示"跳过"
                const isSkipped = !isInSecondPhase && s.name !== '初始化' && s.elapsed_ms < 100;
                
                return (
                  <React.Fragment key={index}>
                    <span style={{ color: isSkipped ? '#999' : '#52c41a', whiteSpace: 'nowrap' }}>
                      {isSkipped ? `○${s.name}(跳过)` : `✓${s.name}(${formatStepTime(s.elapsed_ms)}s)`}
                    </span>
                    {index < progressData.completedSteps!.length - 1 && (
                      <span style={{ color: '#999' }}>→</span>
                    )}
                    {isPhaseBreak && (
                      <>
                        <span style={{ color: '#333', fontWeight: 500 }}>用户选择</span>
                        <span style={{ color: '#999' }}>→</span>
                      </>
                    )}
                  </React.Fragment>
                );
              })}
            </div>
            <div style={{ marginTop: 4, color: '#666' }}>
              总耗时: {formatStepTime(totalMs)}s
            </div>
          </div>
        );
      }
      
      notification.success({
        message: '配置已应用',
        description: notificationDescription,
        duration: 10,  // 显示 10 秒
        placement: 'topRight',
      });
      
      setPreviewOpen(false);
    }
  }, [generatedConfig, mergedConfig, currentMode, onGenerate, finalProgress]);

  const openPreview = useCallback(() => {
    if (generatedConfig) {
      setPreviewOpen(true);
    }
  }, [generatedConfig]);

  const renderConfigValue = useCallback((value: unknown): React.ReactNode => {
    if (value === null || value === undefined) {
      return <span className={styles.nullValue}>null</span>;
    }
    if (typeof value === 'boolean') {
      return <span className={styles.boolValue}>{value ? 'true' : 'false'}</span>;
    }
    if (typeof value === 'number') {
      return <span className={styles.numberValue}>{value}</span>;
    }
    if (typeof value === 'string') {
      // 检测占位符格式：<YOUR_...>、<...>、{{...}}、${...}
      const placeholderPattern = /<YOUR_[A-Za-z0-9_-]+>|<[A-Z][A-Za-z0-9_-]*>|\{\{[A-Za-z0-9_-]+\}\}|\$\{[A-Za-z0-9_-]+\}/;
      if (value.match(placeholderPattern)) {
        return <span className={styles.placeholderValue}>{value}</span>;
      }
      return <span className={styles.stringValue}>"{value}"</span>;
    }
    if (Array.isArray(value)) {
      if (value.length === 0) return <span className={styles.emptyArray}>[]</span>;
      return (
        <div className={styles.arrayValue}>
          [{value.map((item, i) => (
            <div key={i} className={styles.arrayItem}>
              {renderConfigValue(item)}{i < value.length - 1 && ','}
            </div>
          ))}]
        </div>
      );
    }
    if (typeof value === 'object') {
      const entries = Object.entries(value as Record<string, unknown>);
      if (entries.length === 0) return <span className={styles.emptyObject}>{'{}'}</span>;
      return (
        <div className={styles.objectValue}>
          {'{'}
          {entries.map(([k, v], i) => (
            <div key={k} className={styles.objectEntry}>
              <span className={styles.objectKey}>{k}</span>: {renderConfigValue(v)}
              {i < entries.length - 1 && ','}
            </div>
          ))}
          {'}'}
        </div>
      );
    }
    return String(value);
  }, []);

  return {
    expanded,
    description,
    loading,
    generateMode,
    hasCurrentData,
    previewOpen,
    generatedConfig,
    mergedConfig,
    placeholders,
    emptyFields,
    hasGeneratedConfig: !!generatedConfig,
    blockMessage,
    // CMDB 模式
    cmdbMode,
    cmdbLookups,
    warnings,
    needSelection,
    // 进度（从 SSE 获取）
    progress,
    finalProgress,
    // 操作
    setExpanded,
    setDescription,
    setCmdbMode,
    handleGenerate,
    handleGenerateWithSelections,
    handleApplyConfig,
    setPreviewOpen,
    openPreview,
    renderConfigValue,
  };
};

// AI 助手触发按钮组件
export interface AITriggerButtonProps {
  expanded: boolean;
  onClick: () => void;
  disabled?: boolean;
}

export const AITriggerButton: React.FC<AITriggerButtonProps> = ({
  expanded,
  onClick,
  disabled = false,
}) => {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`${styles.triggerButton} ${expanded ? styles.triggerButtonActive : ''}`}
    >
      <AIAssistantIcon className={styles.aiIcon} />
      <span>AI Assistant</span>
    </button>
  );
};

// 弹窗中的进度显示组件（带动态计时）
const ModalProgressDisplay: React.FC<{ progress: ProgressInfo }> = ({ progress }) => {
  // 实时计时器（用于显示当前步骤的动态耗时）
  const [elapsedSeconds, setElapsedSeconds] = React.useState(0);
  const timerRef = React.useRef<ReturnType<typeof setInterval> | null>(null);
  
  // 实时计时器逻辑
  React.useEffect(() => {
    // 每次步骤变化时，重置计时器为 0
    setElapsedSeconds(0);
    
    // 清除之前的计时器
    if (timerRef.current) {
      clearInterval(timerRef.current);
    }
    
    // 开始新的计时
    timerRef.current = setInterval(() => {
      setElapsedSeconds(prev => prev + 1);
    }, 1000);
    
    return () => {
      if (timerRef.current) {
        clearInterval(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [progress.step]);
  
  return (
    <div style={{ marginTop: '12px' }}>
      <div style={{ 
        display: 'flex', 
        alignItems: 'center', 
        justifyContent: 'center',
        gap: 8,
        flexWrap: 'wrap',
        fontSize: 12,
        color: '#666'
      }}>
        {/* 已完成的步骤（绿色） */}
        {progress.completedSteps?.map((step, index) => (
          <span key={index} style={{ display: 'inline-flex', alignItems: 'center', color: '#52c41a' }}>
            ✓{step.name}({formatStepTime(step.elapsed_ms)}s)
            <span style={{ margin: '0 4px', color: '#999' }}>→</span>
          </span>
        ))}
        {/* 当前步骤（蓝色，带动态计时） */}
        <span style={{ color: '#1890ff', fontWeight: 500 }}>
          {progress.stepName}({elapsedSeconds}s)...
        </span>
      </div>
    </div>
  );
};

// CMDB 查询结果展示组件
const CMDBLookupsDisplay: React.FC<{
  lookups: CMDBLookupResult[];
  needSelection: boolean;
  warnings: string[];
  userSelections: Record<string, string | string[]>;  // 支持单选和多选
  onSelectionChange: (targetField: string, resourceId: string | string[]) => void;
  collapsed?: boolean;  // 是否折叠（受控）
  onCollapseChange?: (collapsed: boolean) => void;  // 折叠状态变化回调
}> = ({ lookups, needSelection, warnings, userSelections, onSelectionChange, collapsed = false, onCollapseChange }) => {
  // 计算 activeKeys
  const activeKeys = collapsed ? [] : ['cmdb'];
  
  if (lookups.length === 0 && warnings.length === 0) return null;

  return (
    <div style={{ marginBottom: 16 }}>
      {/* CMDB 查询结果 */}
      {lookups.length > 0 && (
        <Collapse
          activeKey={activeKeys}
          onChange={(keys) => onCollapseChange?.(keys.length === 0)}
          items={[{
            key: 'cmdb',
            label: (
              <span>
                <DatabaseOutlined style={{ marginRight: 8 }} />
                CMDB 查询结果 ({lookups.length} 项)
              </span>
            ),
            children: (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {lookups.map((lookup, index) => {
                  // 从 lookup 中获取 target_field（如果有的话）
                  const targetField = (lookup as any).target_field || lookup.resource_type;
                  // 判断是否是数组类型字段（如 security_group_ids）
                  const isArrayField = targetField.includes('_ids') || targetField.includes('security_group');
                  const selectedValue = userSelections[targetField] || 
                    (isArrayField && lookup.candidates ? lookup.candidates.map(c => c.id) : lookup.candidates?.[0]?.id) || 
                    lookup.result?.id;
                  
                  return (
                    <div 
                      key={index} 
                      style={{ 
                        padding: '8px 12px', 
                        background: lookup.found ? '#f6ffed' : '#fff2f0',
                        borderRadius: 6,
                        border: `1px solid ${lookup.found ? '#b7eb8f' : '#ffccc7'}`
                      }}
                    >
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                        <Tag color={lookup.found ? 'success' : 'error'}>
                          {lookup.found ? '已找到' : '未找到'}
                        </Tag>
                        <span style={{ fontWeight: 500 }}>{lookup.query}</span>
                        <span style={{ color: '#999', fontSize: 12 }}>({lookup.resource_type})</span>
                      </div>
                      
                      {/* 单个结果 - 只有一个候选时显示详情 */}
                      {lookup.found && lookup.result && (!lookup.candidates || lookup.candidates.length <= 1) && (
                        <div style={{ fontSize: 12, color: '#666', marginLeft: 8 }}>
                          <div><strong>ID:</strong> <code>{lookup.result.id}</code></div>
                          <div><strong>名称:</strong> {lookup.result.name}</div>
                          {lookup.result.workspace_name && (
                            <div><strong>来源:</strong> {lookup.result.workspace_name}</div>
                          )}
                        </div>
                      )}
                      
                      {/* 多选情况 - 数组字段默认全选，非数组字段默认选第一个 */}
                      {lookup.found && lookup.candidates && lookup.candidates.length > 1 && (
                        <div style={{ marginTop: 8 }}>
                          <div style={{ fontSize: 12, color: isArrayField ? '#1890ff' : '#faad14', marginBottom: 4 }}>
                            {isArrayField ? (
                              <>
                                <CheckCircleOutlined style={{ marginRight: 4 }} />
                                找到 {lookup.candidates.length} 个匹配资源，可多选：
                              </>
                            ) : (
                              <>
                                <WarningOutlined style={{ marginRight: 4 }} />
                                找到 {lookup.candidates.length} 个匹配资源，请选择：
                              </>
                            )}
                          </div>
                          <Select
                            mode={isArrayField ? 'multiple' : undefined}
                            style={{ width: '100%' }}
                            value={selectedValue}
                            onChange={(value) => onSelectionChange(targetField, value)}
                            options={lookup.candidates.map(c => ({
                              value: c.id,
                              label: `${c.name} (${c.id})${c.workspace_name ? ` - ${c.workspace_name}` : ''}`,
                            }))}
                          />
                        </div>
                      )}
                      
                      {!lookup.found && lookup.error && (
                        <div style={{ fontSize: 12, color: '#ff4d4f', marginLeft: 8 }}>
                          {lookup.error}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            ),
          }]}
        />
      )}
      
      {/* 警告信息 */}
      {warnings.length > 0 && (
        <Alert
          type="warning"
          showIcon
          style={{ marginTop: 8 }}
          message="部分资源未找到"
          description={
            <ul style={{ margin: 0, paddingLeft: 20 }}>
              {warnings.map((w, i) => (
                <li key={i}>{w}</li>
              ))}
            </ul>
          }
        />
      )}
      
      {/* 需要选择提示 */}
      {needSelection && (
        <Alert
          type="info"
          showIcon
          style={{ marginTop: 8 }}
          message="需要您的选择"
          description="找到多个匹配的资源，请在上方选择正确的资源后重新生成配置"
        />
      )}
    </div>
  );
};

// 预览弹窗组件
export interface AIPreviewModalProps {
  open: boolean;
  onClose: () => void;
  onApply: () => void;
  onRecheck?: () => void;  // 强制重新检查
  onRegenerate?: (userSelections: Record<string, string | string[]>) => void;  // 重新生成配置（用于 CMDB 多选后）
  generatedConfig: Record<string, unknown> | null;
  placeholders: PlaceholderInfo[];
  emptyFields?: EmptyFieldInfo[];
  renderConfigValue: (value: unknown) => React.ReactNode;
  mode?: GenerateMode;  // 生成模式
  loading?: boolean;    // 是否正在加载
  blockMessage?: string | null;  // 拦截消息
  userDescription?: string;  // 用户输入的描述
  // CMDB 相关
  cmdbLookups?: CMDBLookupResult[];
  warnings?: string[];
  needSelection?: boolean;
  // 进度（用于显示执行摘要和实时进度）
  progress?: ProgressInfo | null;
  finalProgress?: ProgressInfo | null;
}

export const AIPreviewModal: React.FC<AIPreviewModalProps> = ({
  open,
  onClose,
  onApply,
  onRecheck,
  onRegenerate,
  generatedConfig,
  placeholders: backendPlaceholders,
  emptyFields = [],
  renderConfigValue,
  mode = 'new',
  loading = false,
  blockMessage = null,
  userDescription = '',
  cmdbLookups = EMPTY_CMDB_LOOKUPS,  // 使用稳定的常量引用，避免无限循环
  warnings = EMPTY_WARNINGS,          // 使用稳定的常量引用，避免无限循环
  needSelection = false,
  progress = null,
  finalProgress = null,
}) => {
  const [copied, setCopied] = useState(false);
  
  // 用户选择的资源（用于 CMDB 多选情况，支持单选和多选）
  const [userSelections, setUserSelections] = useState<Record<string, string | string[]>>({});
  
  // CMDB 查询结果是否折叠（点击"使用该资源生成配置"后折叠）
  const [cmdbCollapsed, setCmdbCollapsed] = useState(false);
  
  // 处理用户选择变化（支持单选和多选）
  const handleSelectionChange = useCallback((targetField: string, resourceId: string | string[]) => {
    setUserSelections(prev => ({
      ...prev,
      [targetField]: resourceId,
    }));
  }, []);
  
  // 初始化默认选择（当 cmdbLookups 变化时）
  React.useEffect(() => {
    const defaultSelections: Record<string, string | string[]> = {};
    cmdbLookups.forEach(lookup => {
      const targetField = (lookup as any).target_field || lookup.resource_type;
      // 判断是否是数组类型字段（如 security_group_ids）
      const isArrayField = targetField.includes('_ids') || targetField.includes('security_group');
      
      if (lookup.candidates && lookup.candidates.length > 0) {
        if (isArrayField) {
          // 数组字段默认全选
          defaultSelections[targetField] = lookup.candidates.map(c => c.id);
        } else {
          // 非数组字段默认选第一个
          defaultSelections[targetField] = lookup.candidates[0].id;
        }
      } else if (lookup.result) {
        defaultSelections[targetField] = lookup.result.id;
      }
    });
    setUserSelections(defaultSelections);
    
    // 当有新的 CMDB 查询结果时，重置折叠状态为展开
    if (cmdbLookups.length > 0) {
      setCmdbCollapsed(false);
    }
  }, [cmdbLookups]);
  
  // 基于显示的配置（generatedConfig，实际上是 mergedConfig）重新检测占位符
  // 这样可以检测到用户原有数据中的占位符，而不仅仅是 AI 生成的
  const displayPlaceholders = React.useMemo(() => {
    if (!generatedConfig) return backendPlaceholders;
    const detected = detectPlaceholdersInConfig(generatedConfig);
    // 合并后端返回的和前端检测的，去重
    const allPlaceholders = [...backendPlaceholders];
    detected.forEach(d => {
      if (!allPlaceholders.some(p => p.field === d.field && p.placeholder === d.placeholder)) {
        allPlaceholders.push(d);
      }
    });
    return allPlaceholders;
  }, [generatedConfig, backendPlaceholders]);
  
  const hasWarnings = displayPlaceholders.length > 0 || emptyFields.length > 0;
  const isRefineMode = mode === 'refine';
  const isConfigValid = isRefineMode && !hasWarnings;  // 只有修复模式且没有警告才认为配置符合预期

  const handleCopyJson = useCallback(() => {
    if (generatedConfig) {
      const jsonStr = JSON.stringify(generatedConfig, null, 2);
      navigator.clipboard.writeText(jsonStr).then(() => {
        setCopied(true);
        message.success('已复制到剪贴板');
        setTimeout(() => setCopied(false), 2000);
      }).catch(() => {
        message.error('复制失败');
      });
    }
  }, [generatedConfig]);
  
  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width={600}
      closable={false}
      centered
      className={styles.modal}
      styles={{ 
        content: { padding: 0, borderRadius: 12, overflow: 'hidden' },
        body: { padding: 0 }
      }}
    >
      <div className={styles.modalHeader}>
        <div className={styles.modalTitle}>
          <EyeOutlined />
          <span>配置预览</span>
        </div>
        <button className={styles.closeButton} onClick={onClose}>
          <CloseOutlined />
        </button>
      </div>

      <div className={styles.modalBody}>
        {/* 用户输入的描述 - 始终显示在最上方 */}
        {userDescription && (
          <div style={{ 
            padding: '12px 16px', 
            background: '#f5f5f5', 
            borderRadius: 8, 
            marginBottom: 12,
            fontSize: 14,
            color: '#333',
            borderLeft: '4px solid #1890ff'
          }}>
            <div style={{ fontWeight: 500, marginBottom: 4, color: '#666', fontSize: 12 }}>您的需求：</div>
            {userDescription}
          </div>
        )}

        {/* 执行摘要 - 显示各步骤耗时 */}
        {finalProgress?.completedSteps && finalProgress.completedSteps.length > 0 && (
          <div style={{ 
            padding: '8px 12px', 
            background: '#e6f7ff', 
            borderRadius: 6, 
            marginBottom: 12,
            fontSize: 12,
            color: '#1890ff',
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            flexWrap: 'wrap'
          }}>
            <span style={{ fontWeight: 500 }}>执行摘要：</span>
            {finalProgress.completedSteps.map((step, index) => {
              // 检查是否是两步流程的分界点（第一步的最后一个步骤后面是第二步的初始化）
              const nextStep = finalProgress.completedSteps![index + 1];
              const isPhaseBreak = nextStep?.name === '初始化' && index > 0;
              
              // 检查当前步骤是否在第二步流程中（在"用户选择"分界点之后）
              const phaseBreakIndex = finalProgress.completedSteps!.findIndex((s, i) => {
                const next = finalProgress.completedSteps![i + 1];
                return next?.name === '初始化' && i > 0;
              });
              const isInSecondPhase = phaseBreakIndex >= 0 && index > phaseBreakIndex;
              
              // 只有在第一步流程中，非初始化步骤如果时间小于 100ms 才显示"跳过"
              // 如果步骤有 used_skills，说明确实执行了，不应该显示"跳过"
              const hasUsedSkills = step.used_skills && step.used_skills.length > 0;
              const isSkipped = !isInSecondPhase && step.name !== '初始化' && step.elapsed_ms < 100 && !hasUsedSkills;
              
              // 构建 Tooltip 内容（显示使用的 Skills）
              const skillsTooltip = hasUsedSkills 
                ? (
                  <div style={{ fontSize: 11 }}>
                    <div style={{ fontWeight: 500, marginBottom: 4 }}>使用的 Skills:</div>
                    {step.used_skills!.map((skill, i) => (
                      <div key={i} style={{ color: '#52c41a' }}>• {skill}</div>
                    ))}
                  </div>
                )
                : null;
              
              const stepContent = isSkipped ? (
                <>○{step.name}(跳过)</>
              ) : (
                <>✓{step.name}({formatStepTime(step.elapsed_ms)}s)</>
              );
              
              return (
                <React.Fragment key={index}>
                  {skillsTooltip ? (
                    <Tooltip title={skillsTooltip} placement="top">
                      <span style={{ 
                        display: 'inline-flex', 
                        alignItems: 'center', 
                        color: isSkipped ? '#999' : '#52c41a',
                        cursor: 'help',
                        borderBottom: '1px dashed currentColor'
                      }}>
                        {stepContent}
                      </span>
                    </Tooltip>
                  ) : (
                    <span style={{ 
                      display: 'inline-flex', 
                      alignItems: 'center', 
                      color: isSkipped ? '#999' : '#52c41a' 
                    }}>
                      {stepContent}
                    </span>
                  )}
                  {index < finalProgress.completedSteps!.length - 1 && (
                    isPhaseBreak ? (
                      // 两步流程的分界点，显示"用户选择"
                      <>
                        <span style={{ margin: '0 4px', color: '#999' }}>→</span>
                        <span style={{ color: '#333', fontWeight: 500 }}>用户选择</span>
                        <span style={{ margin: '0 4px', color: '#999' }}>→</span>
                      </>
                    ) : (
                      <span style={{ margin: '0 4px', color: '#999' }}>→</span>
                    )
                  )}
                </React.Fragment>
              );
            })}
            {/* 总耗时单独一行显示 */}
            <div style={{ width: '100%', marginTop: 4, color: '#666', fontSize: 11 }}>
              总耗时: {formatStepTime(finalProgress.completedSteps.reduce((sum, s) => sum + s.elapsed_ms, 0))}s
            </div>
          </div>
        )}

        {blockMessage ? (
          <Alert
            type="error"
            showIcon
            icon={<WarningOutlined />}
            message="请求已被安全系统拦截"
             description={
              <div style={{ whiteSpace: 'pre-line', marginTop: 8 }}>
                {blockMessage}
              </div>
            }
            className={styles.previewAlert}
          />
        ) : loading ? (
          <div style={{ textAlign: 'center', padding: '20px' }}>
            <Spin />
            {progress ? (
              // 显示真实的 SSE 进度
              <ModalProgressDisplay progress={progress} />
            ) : (
              <p style={{ marginTop: '12px', color: '#666' }}>AI 正在重新检查配置...</p>
            )}
          </div>
        ) : cmdbLookups.length > 0 ? (
          <Alert
            type={needSelection ? 'info' : 'success'}
            showIcon
            icon={needSelection ? <DatabaseOutlined /> : <CheckCircleOutlined />}
            message="智能生成完成"
            description={
              <div>
                根据您的描述，我们查询到如下信息：
                {needSelection && (
                  <div style={{ marginTop: 8, color: '#1890ff', fontWeight: 500 }}>
                    存在多个匹配结果，请在下方选择正确的资源后点击「使用该资源生成配置」
                  </div>
                )}
              </div>
            }
            className={styles.previewAlert}
          />
        ) : hasWarnings ? (
          <Alert
            type="warning"
            showIcon
            icon={<WarningOutlined />}
            message="配置已生成，但需要补充信息"
            description={
              <span>
                {displayPlaceholders.length > 0 && `${displayPlaceholders.length} 个占位符需要替换`}
                {displayPlaceholders.length > 0 && emptyFields.length > 0 && '，'}
                {emptyFields.length > 0 && `${emptyFields.length} 个字段值为空`}
              </span>
            }
            className={styles.previewAlert}
          />
        ) : isConfigValid ? (
          <Alert
            type="success"
            showIcon
            icon={<CheckCircleOutlined />}
            message="配置已符合预期"
            description={
              <div>
                <span>AI 分析后认为当前配置已经完整且合理，可以直接应用。</span>
                {onRecheck && (
                  <Button 
                    type="link" 
                    size="small" 
                    onClick={onRecheck}
                    style={{ padding: '0 4px', height: 'auto' }}
                  >
                    强制重新检查
                  </Button>
                )}
              </div>
            }
            className={styles.previewAlert}
          />
        ) : (
          <Alert
            type="success"
            showIcon
            icon={<CheckCircleOutlined />}
            message="配置生成完成"
            description={'请预览以下配置，确认后点击"应用配置"按钮'}
            className={styles.previewAlert}
          />
        )}

        {/* CMDB 查询结果展示 - 默认展开，只有点击"使用该资源生成配置"后才折叠 */}
        {(cmdbLookups.length > 0 || warnings.length > 0) && (
          <CMDBLookupsDisplay 
            lookups={cmdbLookups} 
            needSelection={false}  // 不再在这里显示"需要选择"提示，已合并到上方 Alert
            warnings={warnings}
            userSelections={userSelections}
            onSelectionChange={handleSelectionChange}
            collapsed={cmdbCollapsed}
            onCollapseChange={setCmdbCollapsed}
          />
        )}

        <Tabs
          defaultActiveKey="preview"
          className={styles.previewTabs}
          items={[
            {
              key: 'preview',
              label: <span><EyeOutlined /> 预览</span>,
              children: (
                <div className={styles.configPreview}>
                  {generatedConfig && Object.keys(generatedConfig).length > 0 ? (
                    <div className={styles.configList}>
                      {Object.entries(generatedConfig).map(([key, value]) => (
                        <div key={key} className={styles.configItem}>
                          <span className={styles.configKey}>{key}</span>
                          <span className={styles.configSeparator}>:</span>
                          <span className={styles.configValue}>{renderConfigValue(value)}</span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className={styles.emptyConfig}>没有生成配置</div>
                  )}
                </div>
              ),
            },
            {
              key: 'json',
              label: <span><CodeOutlined /> JSON</span>,
              children: (
                <div className={styles.jsonPreviewContainer}>
                  <button 
                    className={styles.copyButton}
                    onClick={handleCopyJson}
                    disabled={!generatedConfig}
                  >
                    {copied ? (
                      <>
                        <CheckOutlined style={{ fontSize: 12 }} />
                        <span>Copied</span>
                      </>
                    ) : (
                      <>
                        <CopyOutlined style={{ fontSize: 12 }} />
                        <span>Copy</span>
                      </>
                    )}
                  </button>
                  <pre className={styles.jsonPreview}>
                    {JSON.stringify(generatedConfig, null, 2)}
                  </pre>
                </div>
              ),
            },
          ]}
        />

        {displayPlaceholders.length > 0 && (
          <Collapse
            className={styles.placeholderCollapse}
            items={[{
              key: 'placeholders',
              label: `需要补充 ${displayPlaceholders.length} 个占位符`,
              children: (
                <div className={styles.placeholderList}>
                  {displayPlaceholders.map((p: PlaceholderInfo, index: number) => (
                    <div key={index} className={styles.placeholderItem}>
                      <div className={styles.placeholderHeader}>
                        <code className={styles.fieldName}>{p.field}</code>
                        {p.help_link && (
                          <a href={p.help_link} target="_blank" rel="noopener noreferrer" className={styles.helpLink}>
                            <QuestionCircleOutlined /> 帮助
                          </a>
                        )}
                      </div>
                      <p className={styles.fieldDesc}>{p.description}</p>
                    </div>
                  ))}
                </div>
              ),
            }]}
          />
        )}

        {emptyFields.length > 0 && (
          <Collapse
            className={styles.placeholderCollapse}
            defaultActiveKey={['emptyFields']}
            items={[{
              key: 'emptyFields',
              label: <span style={{ color: '#faad14' }}>{emptyFields.length} 个字段值为空</span>,
              children: (
                <div className={styles.placeholderList}>
                  {emptyFields.map((f, index) => (
                    <div key={index} className={styles.placeholderItem} style={{ borderLeftColor: '#faad14' }}>
                      <div className={styles.placeholderHeader}>
                        <code className={styles.fieldName}>{f.field}</code>
                      </div>
                      <p className={styles.fieldDesc}>{f.description}</p>
                    </div>
                  ))}
                </div>
              ),
            }]}
          />
        )}
      </div>

      <div className={styles.modalFooter}>
        <Button onClick={onClose}>取消</Button>
        {needSelection ? (
          <Button 
            type="primary" 
            onClick={() => {
              setCmdbCollapsed(true);  // 折叠 CMDB 查询结果
              onRegenerate?.(userSelections);
            }}
          >
            使用该资源生成配置
          </Button>
        ) : (
          <Button 
            type="primary" 
            onClick={onApply} 
            disabled={!generatedConfig}
          >
            应用配置
          </Button>
        )}
      </div>
    </Modal>
  );
};

// 默认导出：完整的 AI 配置生成器组件（向后兼容）
interface AIConfigGeneratorProps {
  moduleId: number;
  workspaceId?: string;
  organizationId?: string;
  manifestId?: string;
  onGenerate: (config: Record<string, unknown>) => void;
  currentFormData?: Record<string, unknown>;
  disabled?: boolean;
}

const AIConfigGenerator: React.FC<AIConfigGeneratorProps> = ({
  moduleId,
  workspaceId,
  organizationId,
  manifestId,
  onGenerate,
  currentFormData,
  disabled = false,
}) => {
  const ai = useAIConfigGenerator({
    moduleId,
    workspaceId,
    organizationId,
    manifestId,
    currentFormData,
    onGenerate,
  });

  return (
    <>
      <AITriggerButton
        expanded={ai.expanded}
        onClick={() => ai.setExpanded(!ai.expanded)}
        disabled={disabled}
      />

      {ai.expanded && (
        <AIInputPanel
          description={ai.description}
          onDescriptionChange={ai.setDescription}
          onGenerate={ai.handleGenerate}
          onClose={() => ai.setExpanded(false)}
          loading={ai.loading}
          generateMode={ai.generateMode}
          hasCurrentData={ai.hasCurrentData}
          cmdbMode={ai.cmdbMode}
          onCmdbModeChange={ai.setCmdbMode}
          progress={ai.progress}
          finalProgress={ai.finalProgress}
        />
      )}

      <AIPreviewModal
        open={ai.previewOpen}
        onClose={() => ai.setPreviewOpen(false)}
        onApply={ai.handleApplyConfig}
        onRegenerate={ai.handleGenerateWithSelections}
        generatedConfig={ai.mergedConfig || ai.generatedConfig}
        placeholders={ai.placeholders}
        emptyFields={ai.emptyFields}
        renderConfigValue={ai.renderConfigValue}
        mode={ai.generateMode}
        loading={ai.loading}
        blockMessage={ai.blockMessage}
        userDescription={ai.description}
        cmdbLookups={ai.cmdbLookups}
        warnings={ai.warnings}
        needSelection={ai.needSelection}
        progress={ai.progress}
        finalProgress={ai.finalProgress}
      />
    </>
  );
};

export default AIConfigGenerator;
