import { useState, useEffect, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  getAIConfig,
  createAIConfig,
  updateAIConfig,
  deleteAIConfig,
  getAvailableRegions,
  getAvailableModels,
  type AIConfig as AIConfigType,
  type BedrockModel,
  CAPABILITIES,
  CAPABILITY_LABELS,
  CAPABILITY_DESCRIPTIONS,
  DEFAULT_CAPABILITY_PROMPTS,
} from '../services/ai';
import {
  listSkills,
  previewDomainSkillDiscovery,
  type Skill,
  type SkillComposition,
  type DomainSkillMode,
  type PreviewDiscoveryResponse,
  LAYER_LABELS,
  DOMAIN_SKILL_MODE_LABELS,
  DOMAIN_SKILL_MODE_DESCRIPTIONS,
} from '../services/skill';
import ConfirmDialog from '../components/ConfirmDialog';
import styles from './AIConfigForm.module.css';

const AIConfigForm = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const isEditMode = !!id;

  const [config, setConfig] = useState<AIConfigType | null>(null);
  const [regions, setRegions] = useState<string[]>([]);
  const [models, setModels] = useState<BedrockModel[]>([]);
  const [loading, setLoading] = useState(isEditMode);
  const [saving, setSaving] = useState(false);
  const [loadingModels, setLoadingModels] = useState(false);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [conflictWarning, setConflictWarning] = useState(false);
  const [warningTimestamp, setWarningTimestamp] = useState<number | null>(null);
  const [remainingSeconds, setRemainingSeconds] = useState<number>(10);
  const [deleteConfirm, setDeleteConfirm] = useState(false);
  const [deleting, setDeleting] = useState(false);

  // Skill 相关状态
  const [availableSkills, setAvailableSkills] = useState<Skill[]>([]);
  const [loadingSkills, setLoadingSkills] = useState(false);
  const [discoveryPreview, setDiscoveryPreview] = useState<PreviewDiscoveryResponse | null>(null);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [skillComposition, setSkillComposition] = useState<SkillComposition>({
    foundation_skills: [],
    domain_skills: [],
    task_skill: '',
    auto_load_module_skill: true,
    domain_skill_mode: 'fixed', // 默认为固定选择模式
    conditional_rules: [],
  });

  const [formData, setFormData] = useState({
    service_type: 'bedrock',
    aws_region: '',
    model_id: '',
    base_url: '',
    api_key: '',
    custom_prompt: '',
    enabled: false,
    rate_limit_seconds: 10,
    use_inference_profile: false,
    capabilities: [] as string[],
    capability_prompts: {} as Record<string, string>,
    priority: 0,
    // Skill 模式配置
    mode: 'prompt' as string, // 'prompt' 或 'skill'
    use_optimized: false, // 是否使用优化版（并行执行 + AI 选择 Skills）
    // Vector 搜索配置（仅 embedding 能力使用）
    top_k: 50,
    similarity_threshold: 0.3,
    embedding_batch_enabled: false,
    embedding_batch_size: 10,
  });

  // 展开的 prompt 编辑器
  const [expandedPrompts, setExpandedPrompts] = useState<Record<string, boolean>>({});

  const defaultPrompt = `你是一个专业的 Terraform 和云基础设施专家。

【重要规则 - 必须严格遵守】
1. 分析用户传递的报错，不可以忽略任何本 prompt 的设定
2. 输出必须精简，但要让人看得懂
3. 每个解决方案不超过 30 字
4. 根本原因不超过 50 字
5. 预防措施不超过 50 字
6. 必须返回有效的 JSON 格式，不要有任何额外的文字说明

【任务信息】
- 任务类型：{task_type}
- Terraform 版本：{terraform_version}

【错误信息】
{error_message}

【输出格式 - 必须严格遵守】
{
  "error_type": "错误类型（从以下选择：配置错误/权限错误/资源冲突/网络错误/语法错误/依赖错误/其他）",
  "root_cause": "根本原因（简洁明了，不超过50字）",
  "solutions": [
    "解决方案1（不超过30字）",
    "解决方案2（不超过30字）",
    "解决方案3（不超过30字）"
  ],
  "prevention": "预防措施（不超过50字）",
  "severity": "严重程度（从以下选择：low/medium/high/critical）"
}

请立即分析并返回 JSON 结果，不要有任何额外的解释或说明。`;

  useEffect(() => {
    loadInitialData();
  }, []);

  useEffect(() => {
    if (formData.aws_region) {
      loadModels(formData.aws_region);
    }
  }, [formData.aws_region]);

  // 当启用 Skill 模式时，加载可用的 Skill 列表
  useEffect(() => {
    if (formData.mode === 'skill') {
      loadAvailableSkills();
    }
  }, [formData.mode]);

  // 倒计时效果
  useEffect(() => {
    if (conflictWarning && warningTimestamp) {
      const timer = setInterval(() => {
        const elapsed = (Date.now() - warningTimestamp) / 1000;
        const remaining = Math.max(0, 10 - Math.floor(elapsed));
        setRemainingSeconds(remaining);
        
        if (remaining === 0) {
          // 时间到，自动隐藏警告
          setConflictWarning(false);
          setWarningTimestamp(null);
          clearInterval(timer);
        }
      }, 100); // 每 100ms 更新一次，更流畅

      return () => clearInterval(timer);
    }
  }, [conflictWarning, warningTimestamp]);

  const loadInitialData = async () => {
    try {
      const regionsData = await getAvailableRegions();
      console.log('Regions loaded:', regionsData);
      setRegions(regionsData);

      if (isEditMode && id) {
        setLoading(true);
        const configData = await getAIConfig(parseInt(id));
        setConfig(configData);
        setFormData({
          service_type: configData.service_type,
          aws_region: configData.aws_region || '',
          model_id: configData.model_id,
          base_url: configData.base_url || '',
          api_key: '', // 不从服务器加载 API Key
          custom_prompt: configData.custom_prompt || '',
          enabled: configData.enabled,
          rate_limit_seconds: configData.rate_limit_seconds || 10,
          use_inference_profile: configData.use_inference_profile || false,
          capabilities: configData.capabilities || [],
          capability_prompts: configData.capability_prompts || {},
          priority: configData.priority || 0,
          mode: configData.mode || 'prompt',
          use_optimized: configData.use_optimized || false,
          top_k: configData.top_k || 50,
          similarity_threshold: configData.similarity_threshold || 0.3,
          embedding_batch_enabled: configData.embedding_batch_enabled || false,
          embedding_batch_size: configData.embedding_batch_size || 10,
        });

        // 加载已保存的 skill_composition
        if (configData.skill_composition && typeof configData.skill_composition === 'object') {
          const sc = configData.skill_composition as unknown as Record<string, unknown>;
          setSkillComposition({
            foundation_skills: Array.isArray(sc.foundation_skills) ? sc.foundation_skills : [],
            domain_skills: Array.isArray(sc.domain_skills) ? sc.domain_skills : [],
            task_skill: typeof sc.task_skill === 'string' ? sc.task_skill : '',
            auto_load_module_skill: typeof sc.auto_load_module_skill === 'boolean' ? sc.auto_load_module_skill : true,
            domain_skill_mode: (sc.domain_skill_mode as DomainSkillMode) || 'fixed',
            conditional_rules: Array.isArray(sc.conditional_rules) ? sc.conditional_rules : [],
          });
        }

        if (configData.aws_region) {
          await loadModels(configData.aws_region);
        }
      }
    } catch (error: any) {
      console.error('Load initial data error:', error);
      setMessage({
        type: 'error',
        text: error.response?.data?.message || '加载数据失败',
      });
    } finally {
      setLoading(false);
    }
  };

  const loadModels = async (region: string) => {
    try {
      setLoadingModels(true);
      const modelsData = await getAvailableModels(region);
      setModels(modelsData);
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.message || '加载模型列表失败',
      });
    } finally {
      setLoadingModels(false);
    }
  };

  // 加载可用的 Skill 列表
  const loadAvailableSkills = async () => {
    try {
      setLoadingSkills(true);
      const response = await listSkills({ is_active: true, page_size: 100 });
      setAvailableSkills(response.skills || []);
    } catch (error: any) {
      console.error('加载 Skill 列表失败:', error);
    } finally {
      setLoadingSkills(false);
    }
  };

  // 按层级分组 Skill
  const getSkillsByLayer = (layer: 'foundation' | 'domain' | 'task') => {
    return availableSkills.filter(skill => skill.layer === layer);
  };

  // 预览 Domain Skill 自动发现
  const handlePreviewDiscovery = async () => {
    if (!skillComposition.task_skill) {
      setMessage({ type: 'error', text: '请先选择 Task Skill' });
      return;
    }
    try {
      setLoadingPreview(true);
      const result = await previewDomainSkillDiscovery(skillComposition.task_skill);
      setDiscoveryPreview(result);
    } catch (error: any) {
      console.error('预览失败:', error);
      setMessage({ type: 'error', text: error.response?.data?.error || '预览失败' });
    } finally {
      setLoadingPreview(false);
    }
  };

  // 当 Task Skill 或模式变化时，清除预览
  useEffect(() => {
    setDiscoveryPreview(null);
  }, [skillComposition.task_skill, skillComposition.domain_skill_mode]);

  // 切换 Skill 选择
  const toggleSkillSelection = (skillName: string, layer: 'foundation' | 'domain' | 'task') => {
    if (layer === 'task') {
      // Task 层只能选择一个
      setSkillComposition(prev => ({
        ...prev,
        task_skill: prev.task_skill === skillName ? '' : skillName,
      }));
    } else if (layer === 'foundation') {
      setSkillComposition(prev => ({
        ...prev,
        foundation_skills: prev.foundation_skills.includes(skillName)
          ? prev.foundation_skills.filter(s => s !== skillName)
          : [...prev.foundation_skills, skillName],
      }));
    } else if (layer === 'domain') {
      setSkillComposition(prev => ({
        ...prev,
        domain_skills: prev.domain_skills.includes(skillName)
          ? prev.domain_skills.filter(s => s !== skillName)
          : [...prev.domain_skills, skillName],
      }));
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    try {
      setSaving(true);
      setMessage(null);
      
      // 检查警告是否在 10 秒内有效
      let forceUpdate = false;
      if (conflictWarning && warningTimestamp) {
        const elapsedSeconds = (Date.now() - warningTimestamp) / 1000;
        if (elapsedSeconds <= 10) {
          // 10 秒内，允许强制更新
          forceUpdate = true;
        } else {
          // 超过 10 秒，重置警告状态，需要重新触发
          setConflictWarning(false);
          setWarningTimestamp(null);
          forceUpdate = false;
        }
      }
      
      // 构建提交数据，包含 skill_composition
      const submitData = {
        ...formData,
        // 只有在 Skill 模式下才提交 skill_composition（转换为 Record<string, unknown> 类型）
        skill_composition: formData.mode === 'skill' 
          ? skillComposition as unknown as Record<string, unknown>
          : undefined,
      };

      if (isEditMode && id) {
        await updateAIConfig(parseInt(id), submitData, forceUpdate);
        setMessage({
          type: 'success',
          text: '配置更新成功',
        });
        setConflictWarning(false);
        setWarningTimestamp(null);
      } else {
        await createAIConfig(submitData, forceUpdate);
        setMessage({
          type: 'success',
          text: '配置创建成功',
        });
        setConflictWarning(false);
        setWarningTimestamp(null);
      }
      
      // 延迟跳转，让用户看到成功消息
      setTimeout(() => {
        navigate('/global/settings/ai-configs');
      }, 1000);
    } catch (error: any) {
      console.log('Error caught:', error);
      console.log('Error response:', error.response);
      
      // 提取错误消息 - 支持多种错误格式
      let errorMessage = '保存配置失败';
      if (error.response?.data?.message) {
        errorMessage = error.response.data.message;
      } else if (error.message) {
        errorMessage = error.message;
      }
      
      console.log('Extracted error message:', errorMessage);
      
      // 检查是否是配置冲突错误
      if (errorMessage.includes('已有其他 AI 配置处于启用状态')) {
        setConflictWarning(true);
        setWarningTimestamp(Date.now()); // 记录警告时间
        // 不显示普通错误消息，只显示警告框
        setMessage(null);
      } else {
        setConflictWarning(false);
        setWarningTimestamp(null);
        setMessage({
          type: 'error',
          text: errorMessage,
        });
      }
    } finally {
      setSaving(false);
    }
  };

  const handleRegionChange = (region: string) => {
    setFormData({
      ...formData,
      aws_region: region,
      model_id: '', // 重置模型选择
    });
  };

  const handleDelete = async () => {
    if (!id) return;
    
    try {
      setDeleting(true);
      await deleteAIConfig(parseInt(id));
      setMessage({
        type: 'success',
        text: '配置删除成功',
      });
      setDeleteConfirm(false);
      
      // 延迟跳转
      setTimeout(() => {
        navigate('/global/settings/ai-configs');
      }, 1000);
    } catch (error: any) {
      setMessage({
        type: 'error',
        text: error.response?.data?.message || '删除配置失败',
      });
      setDeleteConfirm(false);
    } finally {
      setDeleting(false);
    }
  };

  // 按 Provider 分组渲染模型选项
  const renderModelOptions = () => {
    // 按 Provider 分组
    const groupedModels: Record<string, BedrockModel[]> = {};
    models.forEach((model) => {
      if (!groupedModels[model.provider]) {
        groupedModels[model.provider] = [];
      }
      groupedModels[model.provider].push(model);
    });

    // 所有模型都可以选择，后端会自动处理 inference profile
    // 显示格式：模型名称 (模型ID)
    return Object.keys(groupedModels).sort().map((provider) => (
      <optgroup key={provider} label={provider}>
        {groupedModels[provider].map((model) => (
          <option key={model.id} value={model.id}>
            {model.name} ({model.id})
          </option>
        ))}
      </optgroup>
    ));
  };

  if (loading) {
    return (
      <div className={styles.container}>
        <div className={styles.loading}>加载中...</div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h1 className={styles.title}>
          {isEditMode ? '编辑 AI 配置' : '新增 AI 配置'}
        </h1>
      </div>

      <form onSubmit={handleSubmit} className={styles.form}>
        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>AI 服务配置</h2>

          <div className={styles.formGroup}>
            <label className={styles.label}>服务类型</label>
            <select
              className={styles.select}
              value={formData.service_type}
              onChange={(e) => setFormData({ 
                ...formData, 
                service_type: e.target.value,
                // 切换服务类型时重置相关字段
                aws_region: '',
                model_id: '',
                base_url: '',
                api_key: '',
              })}
            >
              <option value="bedrock">AWS Bedrock</option>
              <option value="openai">OpenAI</option>
              <option value="azure_openai">Azure OpenAI</option>
              <option value="ollama">Ollama</option>
            </select>
          </div>

          {/* Bedrock 特有字段 */}
          {formData.service_type === 'bedrock' && (
            <>
              <div className={styles.formGroup}>
                <label className={styles.label}>AWS Region</label>
                <select
                  className={styles.select}
                  value={formData.aws_region}
                  onChange={(e) => handleRegionChange(e.target.value)}
                  required
                >
                  <option value="">请选择区域</option>
                  {regions.map((region) => (
                    <option key={region} value={region}>
                      {region}
                    </option>
                  ))}
                </select>
              </div>

              <div className={styles.formGroup}>
                <label className={styles.label}>模型</label>
                <select
                  className={styles.select}
                  value={formData.model_id}
                  onChange={(e) => setFormData({ ...formData, model_id: e.target.value })}
                  disabled={!formData.aws_region || loadingModels}
                  required
                >
                  <option value="">
                    {loadingModels ? '加载中...' : '请选择模型'}
                  </option>
                  {renderModelOptions()}
                </select>
                <div className={styles.hint}>
                  推荐：Claude 3.5 Sonnet（稳定可靠）。部分新模型可能需要额外配置。
                </div>
              </div>
            </>
          )}

          {/* OpenAI Compatible 字段 */}
          {(formData.service_type === 'openai' || 
            formData.service_type === 'azure_openai' || 
            formData.service_type === 'ollama') && (
            <>
              <div className={styles.formGroup}>
                <label className={styles.label}>Base URL</label>
                <input
                  type="url"
                  className={styles.select}
                  value={formData.base_url}
                  onChange={(e) => setFormData({ ...formData, base_url: e.target.value })}
                  placeholder={
                    formData.service_type === 'openai' 
                      ? 'https://api.openai.com/v1'
                      : formData.service_type === 'ollama'
                      ? 'http://localhost:11434/v1'
                      : 'https://your-resource.openai.azure.com'
                  }
                  required
                />
                <div className={styles.hint}>
                  {formData.service_type === 'openai' && 'OpenAI API 基础 URL'}
                  {formData.service_type === 'azure_openai' && 'Azure OpenAI 端点 URL'}
                  {formData.service_type === 'ollama' && 'Ollama 服务地址'}
                </div>
              </div>

              <div className={styles.formGroup}>
                <label className={styles.label}>API Key</label>
                <input
                  type="password"
                  className={styles.select}
                  value={formData.api_key}
                  onChange={(e) => setFormData({ ...formData, api_key: e.target.value })}
                  placeholder={isEditMode ? '留空表示不修改' : '请输入 API Key'}
                  required={!isEditMode}
                />
                <div className={styles.hint}>
                  {isEditMode 
                    ? 'API Key 已加密存储，留空表示不修改' 
                    : 'API Key 将加密存储，查询时不返回'}
                </div>
              </div>

              <div className={styles.formGroup}>
                <label className={styles.label}>模型 ID</label>
                <input
                  type="text"
                  className={styles.select}
                  value={formData.model_id}
                  onChange={(e) => setFormData({ ...formData, model_id: e.target.value })}
                  placeholder={
                    formData.service_type === 'openai' 
                      ? 'gpt-4, gpt-3.5-turbo'
                      : formData.service_type === 'ollama'
                      ? 'llama2, mistral'
                      : 'your-deployment-name'
                  }
                  required
                />
                <div className={styles.hint}>
                  {formData.service_type === 'openai' && '如：gpt-4, gpt-4-turbo, gpt-3.5-turbo'}
                  {formData.service_type === 'azure_openai' && 'Azure 部署名称'}
                  {formData.service_type === 'ollama' && '本地模型名称'}
                </div>
              </div>
            </>
          )}

          <div className={styles.formGroup}>
            <label className={styles.label}>自定义 Prompt（可选）</label>
            <textarea
              className={styles.textarea}
              value={formData.custom_prompt}
              onChange={(e) => setFormData({ ...formData, custom_prompt: e.target.value })}
              placeholder="在此输入补充的 prompt 内容...&#10;例如：额外关注 AWS 中国区域的特殊配置要求"
              rows={4}
            />
            <div className={styles.hint}>
              提示：此内容会追加到默认 prompt 之后
            </div>
          </div>

          <div className={styles.formGroup}>
            <label className={styles.label}>频率限制（秒）</label>
            <input
              type="number"
              className={styles.select}
              value={formData.rate_limit_seconds}
              onChange={(e) => setFormData({ ...formData, rate_limit_seconds: parseInt(e.target.value) || 10 })}
              min="1"
              max="3600"
              required
            />
            <div className={styles.hint}>
              每个用户在此时间内只能分析一次（建议：10-60秒）
            </div>
          </div>

          {/* Bedrock 特有选项 */}
          {formData.service_type === 'bedrock' && (
            <div className={styles.formGroup}>
              <label className={styles.checkboxLabel}>
                <input
                  type="checkbox"
                  checked={formData.use_inference_profile}
                  onChange={(e) => setFormData({ ...formData, use_inference_profile: e.target.checked })}
                />
                <span>使用 Cross-Region Inference Profile</span>
              </label>
              <div className={styles.hint}>
                某些新模型（如 Claude Sonnet 4）需要启用此选项。启用后会减少请求次数，避免 503 错误。
              </div>
            </div>
          )}

          <div className={styles.formGroup}>
            <label className={styles.checkboxLabel}>
              <input
                type="checkbox"
                checked={formData.enabled}
                onChange={(e) => {
                  const isEnabled = e.target.checked;
                  if (isEnabled) {
                    // 启用时，自动设置为默认配置（支持所有场景）
                    setFormData({ ...formData, enabled: true, capabilities: ['*'] });
                  } else {
                    // 禁用时，保持当前的 capabilities
                    setFormData({ ...formData, enabled: false });
                  }
                }}
              />
              <span>设置为default</span>
            </label>
            <div className={styles.hint}>
              注意：启用此配置会自动禁用其他所有 AI 配置（全局唯一），并自动设置为支持所有场景
            </div>
          </div>

          {/* Vector 搜索配置（仅 embedding 能力显示） */}
          {(formData.capabilities.includes('*') || formData.capabilities.includes(CAPABILITIES.EMBEDDING)) && (
            <div style={{ 
              border: '1px solid #1890ff', 
              borderRadius: '8px', 
              padding: '16px', 
              marginBottom: '20px',
              backgroundColor: '#f0f7ff' 
            }}>
              <h3 style={{ margin: '0 0 12px 0', fontSize: '14px', fontWeight: 600, color: '#1890ff' }}>
                Vector 搜索配置（Embedding 专用）
              </h3>
              
              <div className={styles.formGroup} style={{ marginBottom: '12px' }}>
                <label className={styles.label}>Top K（返回结果数量）</label>
                <input
                  type="number"
                  className={styles.select}
                  value={formData.top_k}
                  onChange={(e) => setFormData({ ...formData, top_k: parseInt(e.target.value) || 50 })}
                  min="1"
                  max="200"
                  required
                />
                <div className={styles.hint}>
                  向量搜索返回的最大结果数量（建议：20-100）
                </div>
              </div>

              <div className={styles.formGroup} style={{ marginBottom: '12px' }}>
                <label className={styles.label}>相似度阈值（Similarity Threshold）</label>
                <input
                  type="number"
                  className={styles.select}
                  value={formData.similarity_threshold}
                  onChange={(e) => setFormData({ ...formData, similarity_threshold: parseFloat(e.target.value) || 0.3 })}
                  min="0"
                  max="1"
                  step="0.05"
                  required
                />
                <div className={styles.hint}>
                  只返回相似度大于此阈值的结果（0-1，建议：0.2-0.5，越高越精确但结果越少）
                </div>
              </div>

              <div className={styles.formGroup} style={{ marginBottom: '12px' }}>
                <label className={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={formData.embedding_batch_enabled}
                    onChange={(e) => {
                      const wantToEnable = e.target.checked;
                      if (wantToEnable) {
                        // 检查模型是否支持 batch
                        const isBatchSupported = (() => {
                          if (formData.service_type === 'openai') return true; // OpenAI 全部支持
                          if (formData.service_type === 'bedrock') {
                            // Titan V2 和 Cohere Embed 支持批量
                            return formData.model_id.includes('titan-embed-text-v2') || 
                                   formData.model_id.includes('cohere.embed');
                          }
                          return false;
                        })();
                        
                        if (!isBatchSupported) {
                          setMessage({
                            type: 'error',
                            text: '当前模型不支持 Batch Embedding。支持的模型：OpenAI embedding 系列、Amazon Titan V2、Cohere Embed',
                          });
                          return;
                        }
                      }
                      setFormData({ ...formData, embedding_batch_enabled: wantToEnable });
                    }}
                  />
                  <span>启用 Batch Embedding</span>
                </label>
                <div className={styles.hint}>
                  批量处理多个文本，提升 embedding 生成效率
                  {formData.service_type === 'bedrock' && 
                   !formData.model_id.includes('titan-embed-text-v2') && 
                   !formData.model_id.includes('cohere.embed') && (
                    <span style={{ color: '#ff4d4f', marginLeft: '8px' }}>
                       当前模型不支持批量，请选择 Titan V2 或 Cohere Embed
                    </span>
                  )}
                  {(formData.service_type === 'openai' || 
                    formData.model_id.includes('titan-embed-text-v2') ||
                    formData.model_id.includes('cohere.embed')) && (
                    <span style={{ color: '#52c41a', marginLeft: '8px' }}>
                      ✓ 当前模型支持批量
                    </span>
                  )}
                </div>
                {/* 维度提醒 */}
                {formData.model_id && (
                  <div style={{ 
                    marginTop: '8px', 
                    padding: '8px 12px', 
                    backgroundColor: '#fffbe6', 
                    border: '1px solid #ffe58f',
                    borderRadius: '4px',
                    fontSize: '12px',
                    color: '#ad6800'
                  }}>
                    <strong>维度说明：</strong>
                    {formData.model_id.includes('cohere.embed-v4') && ' Cohere Embed v4 输出 1536 维度'}
                    {formData.model_id.includes('cohere.embed-english-v3') && ' Cohere Embed v3 输出 1024 维度'}
                    {formData.model_id.includes('cohere.embed-multilingual-v3') && ' Cohere Embed v3 输出 1024 维度'}
                    {formData.model_id.includes('titan-embed-text-v2') && ' Titan V2 输出 1024 维度（可配置 256/512/1024）'}
                    {formData.model_id.includes('titan-embed-text-v1') && ' Titan V1 输出 1536 维度'}
                    {formData.model_id.includes('text-embedding-3-small') && ' OpenAI small 输出 1536 维度'}
                    {formData.model_id.includes('text-embedding-3-large') && ' OpenAI large 输出 3072 维度'}
                    <br />
                    <span style={{ color: '#1890ff' }}>
                      当前数据库支持 1536 维度。推荐使用 Cohere Embed v4 或 OpenAI text-embedding-3-small。
                    </span>
                  </div>
                )}
              </div>

              {formData.embedding_batch_enabled && (
                <div className={styles.formGroup} style={{ marginBottom: '0' }}>
                  <label className={styles.label}>批量大小（Batch Size）</label>
                  <input
                    type="number"
                    className={styles.select}
                    value={formData.embedding_batch_size}
                    onChange={(e) => setFormData({ ...formData, embedding_batch_size: parseInt(e.target.value) || 10 })}
                    min="1"
                    max="100"
                    required
                  />
                  <div className={styles.hint}>
                    每批处理的文本数量（建议：10-50，过大可能导致 API 超时）
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Skill 模式配置（form_generation、module_skill_generation、cmdb_query_plan、cmdb_need_assessment、intent_assertion 能力显示） */}
          {(formData.capabilities.includes('*') || 
            formData.capabilities.includes(CAPABILITIES.FORM_GENERATION) ||
            formData.capabilities.includes(CAPABILITIES.MODULE_SKILL_GENERATION) ||
            formData.capabilities.includes(CAPABILITIES.CMDB_QUERY_PLAN) ||
            formData.capabilities.includes(CAPABILITIES.CMDB_NEED_ASSESSMENT) ||
            formData.capabilities.includes(CAPABILITIES.INTENT_ASSERTION)) && (
            <div style={{ 
              border: '1px solid #722ed1', 
              borderRadius: '8px', 
              padding: '16px', 
              marginBottom: '20px',
              backgroundColor: '#f9f0ff' 
            }}>
              <h3 style={{ margin: '0 0 12px 0', fontSize: '14px', fontWeight: 600, color: '#722ed1' }}>
                🧠 Skill 模式配置
                {formData.capabilities.includes(CAPABILITIES.MODULE_SKILL_GENERATION) && 
                 !formData.capabilities.includes(CAPABILITIES.FORM_GENERATION) && 
                 !formData.capabilities.includes('*') && 
                 '（Module Skill 生成专用）'}
                {formData.capabilities.includes(CAPABILITIES.FORM_GENERATION) && 
                 !formData.capabilities.includes(CAPABILITIES.MODULE_SKILL_GENERATION) && 
                 !formData.capabilities.includes('*') && 
                 '（表单生成专用）'}
                {(formData.capabilities.includes('*') || 
                  (formData.capabilities.includes(CAPABILITIES.FORM_GENERATION) && 
                   formData.capabilities.includes(CAPABILITIES.MODULE_SKILL_GENERATION))) && 
                 '（表单生成 & Module Skill 生成）'}
              </h3>
              
              <div className={styles.formGroup} style={{ marginBottom: '0' }}>
                <label className={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    checked={formData.mode === 'skill'}
                    onChange={(e) => setFormData({ ...formData, mode: e.target.checked ? 'skill' : 'prompt' })}
                  />
                  <span>启用 Skill 模式</span>
                </label>
                <div className={styles.hint}>
                  启用后，AI 将使用分层 Skill 系统组装 Prompt，而不是使用固定的 capability_prompts。
                  <br />
                  <span style={{ color: '#722ed1' }}>
                    ✨ Skill 模式支持：基础层（通用知识）+ 领域层（专业知识）+ 任务层（工作流程）
                    {formData.capabilities.includes(CAPABILITIES.FORM_GENERATION) && ' + Module Skill（自动生成）'}
                  </span>
                </div>

                {/* 优化版开关 - 仅在 Skill 模式 + 表单生成场景下显示 */}
                {formData.mode === 'skill' && 
                 (formData.capabilities.includes('*') || formData.capabilities.includes(CAPABILITIES.FORM_GENERATION)) && (
                  <div style={{ 
                    marginTop: '12px', 
                    padding: '12px', 
                    backgroundColor: '#e6fffb', 
                    border: '1px solid #87e8de',
                    borderRadius: '6px'
                  }}>
                    <label className={styles.checkboxLabel}>
                      <input
                        type="checkbox"
                        checked={formData.use_optimized}
                        onChange={(e) => setFormData({ ...formData, use_optimized: e.target.checked })}
                      />
                      <span style={{ fontWeight: 500, color: '#13c2c2' }}>🚀 使用优化版（实验性）</span>
                    </label>
                    <div className={styles.hint} style={{ marginLeft: '24px', marginTop: '4px' }}>
                      启用后，系统将：
                      <ul style={{ margin: '4px 0 0 16px', padding: 0, fontSize: '12px', color: '#666' }}>
                        <li>并行执行 CMDB 查询和 AI 智能选择 Domain Skills</li>
                        <li>AI 根据用户需求自动选择最相关的 Domain Skills（而非使用固定配置）</li>
                        <li>减少不必要的 Skill 加载，提升响应速度和生成质量</li>
                      </ul>
                      <span style={{ color: '#faad14', fontSize: '11px' }}>
                         此功能为实验性功能，仅适用于表单生成场景
                      </span>
                    </div>
                  </div>
                )}
                <div className={styles.hint} style={{ marginTop: '8px' }}>
                  {formData.capabilities.includes(CAPABILITIES.MODULE_SKILL_GENERATION) && (
                    <>
                      <br />
                      <span style={{ color: '#eb2f96' }}>
                        📝 Module Skill 生成默认使用：platform_introduction + output_format_standard + schema_validation_rules + module_skill_generation_workflow
                      </span>
                    </>
                  )}
                </div>
                {formData.mode === 'skill' && (
                  <div style={{ marginTop: '16px' }}>
                    {loadingSkills ? (
                      <div style={{ padding: '20px', textAlign: 'center', color: '#999' }}>
                        加载 Skill 列表中...
                      </div>
                    ) : availableSkills.length === 0 ? (
                      <div style={{ 
                        padding: '16px', 
                        backgroundColor: '#fffbe6', 
                        border: '1px solid #ffe58f',
                        borderRadius: '4px',
                        color: '#ad6800'
                      }}>
                        <strong> 未找到可用的 Skill</strong>
                        <br />
                        请先在「AI Skills」页面创建 Skill，或运行初始化脚本插入默认 Skill。
                      </div>
                    ) : (
                      <>
                        {/* Foundation 层 Skill 选择 */}
                        <div style={{ marginBottom: '16px' }}>
                          <div style={{ 
                            fontWeight: 600, 
                            marginBottom: '8px', 
                            color: '#722ed1',
                            display: 'flex',
                            alignItems: 'center',
                            gap: '8px'
                          }}>
                            <span style={{ 
                              backgroundColor: '#722ed1', 
                              color: 'white', 
                              padding: '2px 8px', 
                              borderRadius: '4px',
                              fontSize: '11px'
                            }}>
                              Foundation
                            </span>
                            基础层 Skill（可多选）
                          </div>
                          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
                            {getSkillsByLayer('foundation').map(skill => (
                              <label
                                key={skill.id}
                                style={{
                                  display: 'flex',
                                  alignItems: 'center',
                                  gap: '6px',
                                  padding: '6px 12px',
                                  backgroundColor: skillComposition.foundation_skills.includes(skill.name) 
                                    ? '#f9f0ff' 
                                    : '#fafafa',
                                  border: skillComposition.foundation_skills.includes(skill.name)
                                    ? '1px solid #722ed1'
                                    : '1px solid #d9d9d9',
                                  borderRadius: '4px',
                                  cursor: 'pointer',
                                  fontSize: '13px',
                                }}
                              >
                                <input
                                  type="checkbox"
                                  checked={skillComposition.foundation_skills.includes(skill.name)}
                                  onChange={() => toggleSkillSelection(skill.name, 'foundation')}
                                />
                                <span>{skill.display_name}</span>
                                <span style={{ color: '#999', fontSize: '11px' }}>({skill.name})</span>
                              </label>
                            ))}
                            {getSkillsByLayer('foundation').length === 0 && (
                              <span style={{ color: '#999', fontSize: '12px' }}>暂无 Foundation 层 Skill</span>
                            )}
                          </div>
                        </div>

                        {/* Domain 层 Skill 选择 */}
                        <div style={{ 
                          marginBottom: '16px',
                          opacity: formData.use_optimized ? 0.5 : 1,
                          pointerEvents: formData.use_optimized ? 'none' : 'auto'
                        }}>
                          <div style={{
                            fontWeight: 600,
                            marginBottom: '8px',
                            color: formData.use_optimized ? '#999' : '#1890ff',
                            display: 'flex',
                            alignItems: 'center',
                            gap: '8px'
                          }}>
                            <span style={{
                              backgroundColor: formData.use_optimized ? '#999' : '#1890ff',
                              color: 'white',
                              padding: '2px 8px',
                              borderRadius: '4px',
                              fontSize: '11px'
                            }}>
                              Domain
                            </span>
                            领域层 Skill
                            {formData.use_optimized && (
                              <span style={{ 
                                fontSize: '12px', 
                                color: '#ff4d4f', 
                                fontWeight: 'normal',
                                marginLeft: '8px'
                              }}>
                                （已启用优化版，由 AI 自动选择）
                              </span>
                            )}
                          </div>

                          {/* Domain Skill 加载模式选择 */}
                          <div style={{ 
                            marginBottom: '12px', 
                            padding: '12px', 
                            backgroundColor: '#f0f7ff', 
                            borderRadius: '6px',
                            border: '1px solid #91d5ff'
                          }}>
                            <div style={{ fontWeight: 500, marginBottom: '8px', fontSize: '13px', color: '#1890ff' }}>
                              加载模式
                            </div>
                            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                              {(['fixed', 'auto', 'hybrid'] as DomainSkillMode[]).map(mode => (
                                <label
                                  key={mode}
                                  style={{
                                    display: 'flex',
                                    alignItems: 'flex-start',
                                    gap: '8px',
                                    padding: '8px 12px',
                                    backgroundColor: skillComposition.domain_skill_mode === mode ? '#e6f7ff' : '#fff',
                                    border: skillComposition.domain_skill_mode === mode ? '1px solid #1890ff' : '1px solid #d9d9d9',
                                    borderRadius: '4px',
                                    cursor: 'pointer',
                                  }}
                                >
                                  <input
                                    type="radio"
                                    name="domain_skill_mode"
                                    checked={skillComposition.domain_skill_mode === mode}
                                    onChange={() => setSkillComposition(prev => ({
                                      ...prev,
                                      domain_skill_mode: mode
                                    }))}
                                    style={{ marginTop: '2px' }}
                                  />
                                  <div>
                                    <div style={{ fontWeight: 500, fontSize: '13px' }}>
                                      {DOMAIN_SKILL_MODE_LABELS[mode]}
                                    </div>
                                    <div style={{ fontSize: '12px', color: '#666', marginTop: '2px' }}>
                                      {DOMAIN_SKILL_MODE_DESCRIPTIONS[mode]}
                                    </div>
                                  </div>
                                </label>
                              ))}
                            </div>
                            {(skillComposition.domain_skill_mode === 'auto' || skillComposition.domain_skill_mode === 'hybrid') && (
                              <div style={{ 
                                marginTop: '10px', 
                                padding: '8px', 
                                backgroundColor: '#fffbe6', 
                                border: '1px solid #ffe58f',
                                borderRadius: '4px',
                                fontSize: '12px',
                                color: '#ad6800'
                              }}>
                                💡 自动发现模式会解析 Task Skill 内容中的 <code>@require-domain</code> 声明
                              </div>
                            )}
                          </div>

                          {/* 固定选择的 Domain Skills（仅 fixed 和 hybrid 模式显示） */}
                          {(skillComposition.domain_skill_mode === 'fixed' || skillComposition.domain_skill_mode === 'hybrid') && (
                            <>
                              <div style={{ fontSize: '13px', fontWeight: 500, marginBottom: '8px', color: '#666' }}>
                                {skillComposition.domain_skill_mode === 'fixed' ? '选择 Domain Skills（可多选）' : '固定选择的 Domain Skills（可多选，自动发现的会补充加载）'}
                              </div>
                              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
                                {getSkillsByLayer('domain').map(skill => (
                                  <label
                                    key={skill.id}
                                    style={{
                                      display: 'flex',
                                      alignItems: 'center',
                                      gap: '6px',
                                      padding: '6px 12px',
                                      backgroundColor: skillComposition.domain_skills.includes(skill.name) 
                                        ? '#e6f7ff' 
                                        : '#fafafa',
                                      border: skillComposition.domain_skills.includes(skill.name)
                                        ? '1px solid #1890ff'
                                        : '1px solid #d9d9d9',
                                      borderRadius: '4px',
                                      cursor: 'pointer',
                                      fontSize: '13px',
                                    }}
                                  >
                                    <input
                                      type="checkbox"
                                      checked={skillComposition.domain_skills.includes(skill.name)}
                                      onChange={() => toggleSkillSelection(skill.name, 'domain')}
                                    />
                                    <span>{skill.display_name}</span>
                                    <span style={{ color: '#999', fontSize: '11px' }}>({skill.name})</span>
                                  </label>
                                ))}
                                {getSkillsByLayer('domain').length === 0 && (
                                  <span style={{ color: '#999', fontSize: '12px' }}>暂无 Domain 层 Skill</span>
                                )}
                              </div>
                            </>
                          )}

                          {/* auto 模式提示和预览 */}
                          {skillComposition.domain_skill_mode === 'auto' && (
                            <div style={{ 
                              padding: '12px', 
                              backgroundColor: '#f6ffed', 
                              border: '1px solid #b7eb8f',
                              borderRadius: '4px',
                              fontSize: '12px',
                              color: '#389e0d'
                            }}>
                              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                <span>✓ 自动发现模式已启用，Domain Skills 将根据 Task Skill 的 domain_tags 自动匹配</span>
                                <button
                                  type="button"
                                  onClick={handlePreviewDiscovery}
                                  disabled={loadingPreview || !skillComposition.task_skill}
                                  style={{
                                    padding: '4px 12px',
                                    fontSize: '12px',
                                    backgroundColor: skillComposition.task_skill ? '#52c41a' : '#ccc',
                                    color: 'white',
                                    border: 'none',
                                    borderRadius: '4px',
                                    cursor: skillComposition.task_skill ? 'pointer' : 'not-allowed',
                                  }}
                                >
                                  {loadingPreview ? '预览中...' : '🔍 预览发现结果'}
                                </button>
                              </div>
                              
                              {/* 预览结果 */}
                              {discoveryPreview && (
                                <div style={{ 
                                  marginTop: '12px', 
                                  padding: '12px', 
                                  backgroundColor: '#fff',
                                  border: '1px solid #d9f7be',
                                  borderRadius: '4px'
                                }}>
                                  <div style={{ fontWeight: 500, marginBottom: '8px', color: '#237804' }}>
                                    📋 预览结果（保存后生效）
                                  </div>
                                  <div style={{ marginBottom: '8px' }}>
                                    <span style={{ color: '#666' }}>Task Skill:</span>{' '}
                                    <span style={{ fontWeight: 500 }}>{discoveryPreview.task_skill}</span>
                                  </div>
                                  <div style={{ marginBottom: '8px' }}>
                                    <span style={{ color: '#666' }}>domain_tags:</span>{' '}
                                    {discoveryPreview.domain_tags.length > 0 ? (
                                      discoveryPreview.domain_tags.map(tag => (
                                        <span key={tag} style={{
                                          display: 'inline-block',
                                          padding: '2px 8px',
                                          margin: '2px 4px 2px 0',
                                          backgroundColor: '#e6f7ff',
                                          border: '1px solid #91d5ff',
                                          borderRadius: '4px',
                                          fontSize: '11px',
                                          color: '#1890ff'
                                        }}>
                                          {tag}
                                        </span>
                                      ))
                                    ) : (
                                      <span style={{ color: '#ff4d4f' }}>（未定义，请在 Task Skill 中添加 domain_tags）</span>
                                    )}
                                  </div>
                                  <div>
                                    <span style={{ color: '#666' }}>将自动发现的 Domain Skills ({discoveryPreview.discovered_count}):</span>
                                    {discoveryPreview.discovered_skills.length > 0 ? (
                                      <div style={{ marginTop: '8px', display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
                                        {discoveryPreview.discovered_skills.map(skill => (
                                          <div key={skill.name} style={{
                                            padding: '6px 10px',
                                            backgroundColor: '#f0f5ff',
                                            border: '1px solid #adc6ff',
                                            borderRadius: '4px',
                                          }}>
                                            <div style={{ fontWeight: 500, fontSize: '12px' }}>{skill.display_name}</div>
                                            <div style={{ fontSize: '11px', color: '#666' }}>{skill.name}</div>
                                            <div style={{ fontSize: '10px', color: '#999', marginTop: '2px' }}>
                                              tags: {skill.tags?.join(', ') || '无'}
                                            </div>
                                          </div>
                                        ))}
                                      </div>
                                    ) : (
                                      <div style={{ marginTop: '8px', color: '#ff4d4f' }}>
                                         未找到匹配的 Domain Skills，请检查 Task Skill 的 domain_tags 配置
                                      </div>
                                    )}
                                  </div>
                                  {discoveryPreview.message && (
                                    <div style={{ marginTop: '8px', color: '#faad14', fontSize: '11px' }}>
                                      💡 {discoveryPreview.message}
                                    </div>
                                  )}
                                </div>
                              )}
                            </div>
                          )}
                        </div>

                        {/* Task 层 Skill 选择 */}
                        <div style={{ marginBottom: '16px' }}>
                          <div style={{ 
                            fontWeight: 600, 
                            marginBottom: '8px', 
                            color: '#52c41a',
                            display: 'flex',
                            alignItems: 'center',
                            gap: '8px'
                          }}>
                            <span style={{ 
                              backgroundColor: '#52c41a', 
                              color: 'white', 
                              padding: '2px 8px', 
                              borderRadius: '4px',
                              fontSize: '11px'
                            }}>
                              Task
                            </span>
                            任务层 Skill（单选）
                          </div>
                          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px' }}>
                            {getSkillsByLayer('task').map(skill => (
                              <label
                                key={skill.id}
                                style={{
                                  display: 'flex',
                                  alignItems: 'center',
                                  gap: '6px',
                                  padding: '6px 12px',
                                  backgroundColor: skillComposition.task_skill === skill.name 
                                    ? '#f6ffed' 
                                    : '#fafafa',
                                  border: skillComposition.task_skill === skill.name
                                    ? '1px solid #52c41a'
                                    : '1px solid #d9d9d9',
                                  borderRadius: '4px',
                                  cursor: 'pointer',
                                  fontSize: '13px',
                                }}
                              >
                                <input
                                  type="radio"
                                  name="task_skill"
                                  checked={skillComposition.task_skill === skill.name}
                                  onChange={() => toggleSkillSelection(skill.name, 'task')}
                                />
                                <span>{skill.display_name}</span>
                                <span style={{ color: '#999', fontSize: '11px' }}>({skill.name})</span>
                              </label>
                            ))}
                            {getSkillsByLayer('task').length === 0 && (
                              <span style={{ color: '#999', fontSize: '12px' }}>暂无 Task 层 Skill</span>
                            )}
                          </div>
                        </div>

                        {/* Module Skill 自动加载选项（仅 form_generation 显示，module_skill_generation 不需要） */}
                        {(formData.capabilities.includes('*') || 
                          formData.capabilities.includes(CAPABILITIES.FORM_GENERATION)) && 
                         !formData.capabilities.includes(CAPABILITIES.MODULE_SKILL_GENERATION) && (
                          <div style={{ marginBottom: '12px' }}>
                            <label style={{ 
                              display: 'flex', 
                              alignItems: 'center', 
                              gap: '8px',
                              cursor: 'pointer'
                            }}>
                              <input
                                type="checkbox"
                                checked={skillComposition.auto_load_module_skill}
                                onChange={(e) => setSkillComposition(prev => ({
                                  ...prev,
                                  auto_load_module_skill: e.target.checked
                                }))}
                              />
                              <span style={{ fontWeight: 500 }}>自动加载 Module Skill</span>
                            </label>
                            <div style={{ marginLeft: '24px', fontSize: '12px', color: '#666' }}>
                              启用后，系统会自动加载当前 Module 生成的专属 Skill（如果存在）
                            </div>
                          </div>
                        )}
                        {/* Module Skill 生成场景提示 */}
                        {formData.capabilities.includes(CAPABILITIES.MODULE_SKILL_GENERATION) && 
                         !formData.capabilities.includes(CAPABILITIES.FORM_GENERATION) &&
                         !formData.capabilities.includes('*') && (
                          <div style={{ 
                            marginBottom: '12px', 
                            padding: '8px 12px',
                            backgroundColor: '#fff7e6',
                            border: '1px solid #ffd591',
                            borderRadius: '4px',
                            fontSize: '12px',
                            color: '#ad6800'
                          }}>
                            💡 Module Skill 生成场景不需要"自动加载 Module Skill"，因为它是用来生成 Module Skill 的。
                          </div>
                        )}

                        {/* 当前选择摘要 */}
                        <div style={{ 
                          marginTop: '16px',
                          padding: '12px', 
                          backgroundColor: '#fff', 
                          border: '1px solid #d3adf7',
                          borderRadius: '4px',
                          fontSize: '12px',
                        }}>
                          <strong style={{ color: '#531dab' }}>📋 当前 Skill 组合配置：</strong>
                          <div style={{ marginTop: '8px', lineHeight: '1.8' }}>
                            <div>
                              <span style={{ color: '#722ed1' }}>Foundation:</span>{' '}
                              {skillComposition.foundation_skills.length > 0 
                                ? skillComposition.foundation_skills.join(', ')
                                : <span style={{ color: '#999' }}>（未选择，将使用默认）</span>
                              }
                            </div>
                            <div>
                              <span style={{ color: '#1890ff' }}>Domain:</span>{' '}
                              {skillComposition.domain_skills.length > 0 
                                ? skillComposition.domain_skills.join(', ')
                                : <span style={{ color: '#999' }}>（未选择，将使用默认）</span>
                              }
                            </div>
                            <div>
                              <span style={{ color: '#52c41a' }}>Task:</span>{' '}
                              {skillComposition.task_skill 
                                ? skillComposition.task_skill
                                : <span style={{ color: '#999' }}>（未选择，将使用默认）</span>
                              }
                            </div>
                            {/* 仅 form_generation 显示 Module Skill 加载状态 */}
                            {(formData.capabilities.includes('*') || 
                              formData.capabilities.includes(CAPABILITIES.FORM_GENERATION)) && 
                             !formData.capabilities.includes(CAPABILITIES.MODULE_SKILL_GENERATION) && (
                              <div>
                                <span style={{ color: '#faad14' }}>Module Skill:</span>{' '}
                                {skillComposition.auto_load_module_skill 
                                  ? '自动加载' 
                                  : '不加载'
                                }
                              </div>
                            )}
                          </div>
                          <div style={{ marginTop: '8px', color: '#999', fontSize: '11px' }}>
                            提示：如果未选择任何 Skill，系统将使用对应能力的默认 Skill 组合
                            {formData.capabilities.includes(CAPABILITIES.MODULE_SKILL_GENERATION) && (
                              <span style={{ display: 'block', marginTop: '4px', color: '#eb2f96' }}>
                                Module Skill 生成默认：module_skill_generation_workflow
                              </span>
                            )}
                          </div>
                        </div>
                      </>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* 能力场景选择 */}
          <div className={styles.formGroup}>
            <label className={styles.label}>支持的能力场景</label>
            <div className={styles.hint} style={{ marginBottom: '12px' }}>
              选择此配置支持的 AI 能力场景，可为每个场景自定义 Prompt
            </div>

            {/* 专用场景选择 */}
            <div style={{ marginBottom: '8px', fontWeight: 500 }}>专用场景（可多选）</div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              {Object.entries(CAPABILITIES).map(([key, value]) => {
                const isChecked = formData.capabilities.includes('*') || formData.capabilities.includes(value);
                const isExpanded = expandedPrompts[value];
                const customPrompt = formData.capability_prompts[value] || '';
                const defaultPromptForCapability = DEFAULT_CAPABILITY_PROMPTS[value] || '';
                
                return (
                  <div key={value} style={{ 
                    border: isChecked ? '1px solid #1890ff' : '1px solid #e8e8e8',
                    borderRadius: '8px',
                    padding: '12px',
                    backgroundColor: isChecked ? '#f0f7ff' : '#fafafa',
                  }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <label className={styles.checkboxLabel} style={{ flex: 1 }}>
                        <input
                          type="checkbox"
                          checked={isChecked}
                          disabled={formData.enabled}
                          onChange={(e) => {
                            if (e.target.checked) {
                              // 添加场景
                              if (formData.capabilities.includes('*')) {
                                const allCapabilities = Object.values(CAPABILITIES);
                                setFormData({
                                  ...formData,
                                  capabilities: allCapabilities,
                                });
                              } else {
                                setFormData({
                                  ...formData,
                                  capabilities: [...formData.capabilities, value],
                                });
                              }
                            } else {
                              // 移除场景
                              if (formData.capabilities.includes('*')) {
                                const allCapabilities = Object.values(CAPABILITIES).filter(c => c !== value);
                                setFormData({
                                  ...formData,
                                  capabilities: allCapabilities,
                                });
                              } else {
                                setFormData({
                                  ...formData,
                                  capabilities: formData.capabilities.filter((c) => c !== value),
                                });
                              }
                            }
                          }}
                        />
                        <span style={{ fontWeight: 500 }}>{CAPABILITY_LABELS[value]}</span>
                      </label>
                      {isChecked && (
                        <button
                          type="button"
                          onClick={() => setExpandedPrompts(prev => ({ ...prev, [value]: !prev[value] }))}
                          style={{
                            padding: '4px 12px',
                            fontSize: '12px',
                            backgroundColor: customPrompt ? '#52c41a' : '#1890ff',
                            color: 'white',
                            border: 'none',
                            borderRadius: '4px',
                            cursor: 'pointer',
                            display: 'flex',
                            alignItems: 'center',
                            gap: '4px',
                          }}
                        >
                          {customPrompt ? '✓ 已自定义' : '编辑 Prompt'}
                          <span style={{ fontSize: '10px' }}>{isExpanded ? '▲' : '▼'}</span>
                        </button>
                      )}
                    </div>
                    <div className={styles.hint} style={{ marginLeft: '24px', marginTop: '4px' }}>
                      {CAPABILITY_DESCRIPTIONS[value]}
                    </div>
                    
                    {/* Prompt 编辑器 */}
                    {isChecked && isExpanded && (
                      <div style={{ marginTop: '12px', paddingTop: '12px', borderTop: '1px dashed #d9d9d9' }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
                          <span style={{ fontSize: '13px', fontWeight: 500, color: '#666' }}>
                            自定义 Prompt（留空使用默认）
                          </span>
                          <div style={{ display: 'flex', gap: '8px' }}>
                            {customPrompt && (
                              <button
                                type="button"
                                onClick={() => {
                                  const newPrompts = { ...formData.capability_prompts };
                                  delete newPrompts[value];
                                  setFormData({ ...formData, capability_prompts: newPrompts });
                                }}
                                style={{
                                  padding: '2px 8px',
                                  fontSize: '11px',
                                  backgroundColor: '#ff4d4f',
                                  color: 'white',
                                  border: 'none',
                                  borderRadius: '3px',
                                  cursor: 'pointer',
                                }}
                              >
                                清除自定义
                              </button>
                            )}
                            <button
                              type="button"
                              onClick={() => {
                                setFormData({
                                  ...formData,
                                  capability_prompts: {
                                    ...formData.capability_prompts,
                                    [value]: defaultPromptForCapability,
                                  },
                                });
                              }}
                              style={{
                                padding: '2px 8px',
                                fontSize: '11px',
                                backgroundColor: '#faad14',
                                color: 'white',
                                border: 'none',
                                borderRadius: '3px',
                                cursor: 'pointer',
                              }}
                            >
                              加载默认模板
                            </button>
                          </div>
                        </div>
                        <textarea
                          value={customPrompt}
                          onChange={(e) => {
                            setFormData({
                              ...formData,
                              capability_prompts: {
                                ...formData.capability_prompts,
                                [value]: e.target.value,
                              },
                            });
                          }}
                          placeholder={`输入 ${CAPABILITY_LABELS[value]} 的自定义 Prompt...\n\n点击"加载默认模板"可以查看和修改默认 Prompt`}
                          style={{
                            width: '100%',
                            minHeight: '200px',
                            padding: '10px',
                            fontSize: '13px',
                            fontFamily: 'Monaco, Consolas, monospace',
                            border: '1px solid #d9d9d9',
                            borderRadius: '4px',
                            resize: 'vertical',
                            lineHeight: '1.5',
                          }}
                        />
                        <div style={{ marginTop: '8px', fontSize: '12px', color: '#999' }}>
                          提示：可使用变量占位符，如 {'{task_type}'}, {'{error_message}'}, {'{terraform_version}'} 等
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
            {!formData.capabilities.includes('*') && formData.capabilities.length === 0 && (
              <div className={styles.hint} style={{ marginTop: '12px' }}>
                提示：不选择任何场景表示"未配置"，该配置不会被使用
              </div>
            )}
          </div>

          <div className={styles.buttonGroup} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div style={{ display: 'flex', gap: '12px' }}>
              <button
                type="submit"
                className={styles.submitButton}
                disabled={saving || (formData.capabilities.length === 0)}
                style={formData.capabilities.length === 0 ? { 
                  backgroundColor: '#ccc', 
                  cursor: 'not-allowed',
                  opacity: 0.6
                } : {}}
              >
                {saving ? '保存中...' : isEditMode ? '更新配置' : '创建配置'}
              </button>
              <button
                type="button"
                className={styles.cancelButton}
                onClick={() => navigate('/global/settings/ai-configs')}
                disabled={saving}
              >
                取消
              </button>
            </div>
            {isEditMode && (
              <button
                type="button"
                onClick={() => setDeleteConfirm(true)}
                disabled={saving}
                style={{
                  padding: '10px 20px',
                  backgroundColor: '#dc3545',
                  color: 'white',
                  border: 'none',
                  borderRadius: '4px',
                  cursor: saving ? 'not-allowed' : 'pointer',
                  fontSize: '14px',
                  fontWeight: 500,
                  opacity: saving ? 0.6 : 1,
                }}
              >
                删除配置
              </button>
            )}
          </div>

          {/* 在按钮下方显示消息和警告 */}
          {message && (
            <div className={`${styles.message} ${styles[message.type]}`} style={{ marginTop: '16px' }}>
              {message.text}
            </div>
          )}

          {conflictWarning && (
            <div className={styles.conflictWarning} style={{ marginTop: '16px' }}>
              <strong> 警告：</strong>检测到其他 AI 配置已启用。
              <br />
              如需继续启用此配置，请在 <strong style={{ color: '#ff6b6b' }}>{remainingSeconds}</strong> 秒内再次点击「{isEditMode ? '更新配置' : '创建配置'}」按钮确认。
              <br />
              <span style={{ fontSize: '0.9em', opacity: 0.9 }}>
                确认后将自动禁用其他配置。超时后需要重新触发警告。
              </span>
            </div>
          )}
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>默认 Prompt（不可修改）</h2>
          <pre className={styles.defaultPrompt}>{defaultPrompt}</pre>
        </div>

        <div className={styles.section}>
          <h2 className={styles.sectionTitle}>使用说明</h2>
          <ul className={styles.usageList}>
            {formData.service_type === 'bedrock' && (
              <>
                <li>AWS Bedrock 使用 IAM 认证</li>
                <li>确保运行环境配置了 AWS 凭证</li>
              </>
            )}
            {(formData.service_type === 'openai' || 
              formData.service_type === 'azure_openai' || 
              formData.service_type === 'ollama') && (
              <>
                <li>支持 OpenAI Compatible API</li>
                <li>API Key 加密存储，查询时不返回</li>
                <li>兼容 OpenAI、Azure OpenAI、Ollama、vLLM 等</li>
              </>
            )}
            <li>可配置频率限制（默认 10 秒）</li>
            <li>分析结果会保存，可重新分析</li>
            <li>仅在任务详情页的错误卡片中显示分析按钮</li>
          </ul>
        </div>
      </form>

      <ConfirmDialog
        isOpen={deleteConfirm}
        title="删除 AI 配置"
        message="确定要删除此配置吗？删除后无法恢复。"
        confirmText="删除"
        cancelText="取消"
        type="danger"
        onConfirm={handleDelete}
        onCancel={() => setDeleteConfirm(false)}
        loading={deleting}
      />
    </div>
  );
};

export default AIConfigForm;
