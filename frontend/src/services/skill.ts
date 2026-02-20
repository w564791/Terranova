import api from './api';

// Skill 层级类型
export type SkillLayer = 'foundation' | 'domain' | 'task';

// Skill 来源类型
export type SkillSourceType = 'manual' | 'module_auto' | 'hybrid';

// Skill 元数据
export interface SkillMetadata {
  tags?: string[];        // 标签（Domain Skill 用于被发现）
  domain_tags?: string[]; // 需要的领域标签（Task Skill 用于发现 Domain Skills）
  description?: string;
  author?: string;
  usage_count?: number;
  avg_rating?: number;
}

// Skill 接口定义
export interface Skill {
  id: string;
  name: string;
  display_name: string;
  description?: string;  // 新增：Skill 描述，用于 AI 智能选择
  layer: SkillLayer;
  content: string;
  version: string;
  is_active: boolean;
  priority: number;
  source_type: SkillSourceType;
  source_module_id?: number;
  metadata?: SkillMetadata;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

// Skill 列表响应
export interface SkillListResponse {
  skills: Skill[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

// 创建 Skill 请求
export interface CreateSkillRequest {
  name: string;
  display_name: string;
  description?: string;  // 新增：Skill 描述
  layer: SkillLayer;
  content: string;
  version?: string;
  priority?: number;
  source_type?: SkillSourceType;
  metadata?: SkillMetadata;
}

// 更新 Skill 请求
export interface UpdateSkillRequest {
  display_name?: string;
  description?: string;  // 新增：Skill 描述
  content?: string;
  version?: string;
  is_active?: boolean;
  priority?: number;
  metadata?: SkillMetadata;
}

// Skill 使用统计
export interface SkillUsageStats {
  skill_id: string;
  skill_name: string;
  usage_count: number;
  avg_rating: number;
  avg_exec_time_ms: number;
  last_used_at?: string;
}

// Domain Skill 加载模式
export type DomainSkillMode = 'fixed' | 'auto' | 'hybrid';

// Domain Skill 模式标签映射
export const DOMAIN_SKILL_MODE_LABELS: Record<DomainSkillMode, string> = {
  fixed: '固定选择',
  auto: '自动发现',
  hybrid: '混合模式',
};

// Domain Skill 模式描述
export const DOMAIN_SKILL_MODE_DESCRIPTIONS: Record<DomainSkillMode, string> = {
  fixed: '只使用手动选择的 Domain Skills（推荐小规模场景）',
  auto: '从 Task Skill 内容中自动发现依赖（推荐大规模场景）',
  hybrid: '固定选择 + 自动发现补充',
};

// Skill 组合配置
export interface SkillComposition {
  foundation_skills: string[];
  domain_skills: string[];
  task_skill: string;
  auto_load_module_skill: boolean;
  domain_skill_mode?: DomainSkillMode; // 新增：Domain Skill 加载模式
  conditional_rules?: ConditionalRule[];
}

// 条件规则
export interface ConditionalRule {
  condition: string;
  add_skills: string[];
}

// 层级标签映射
export const LAYER_LABELS: Record<SkillLayer, string> = {
  foundation: '基础层',
  domain: '领域层',
  task: '任务层',
};

// 来源类型标签映射
export const SOURCE_TYPE_LABELS: Record<SkillSourceType, string> = {
  manual: '手动创建',
  module_auto: 'Module 自动',
  hybrid: '已编辑',
};

// 获取 Skill 列表
export const listSkills = async (params?: {
  layer?: SkillLayer;
  source_type?: SkillSourceType;
  is_active?: boolean;
  search?: string;
  page?: number;
  page_size?: number;
}): Promise<SkillListResponse> => {
  // api 拦截器已经返回 response.data，所以直接返回
  return await api.get('/admin/skills', { params }) as unknown as SkillListResponse;
};

// 获取单个 Skill
export const getSkill = async (id: string): Promise<Skill> => {
  return await api.get(`/admin/skills/${id}`) as unknown as Skill;
};

// 创建 Skill
export const createSkill = async (data: CreateSkillRequest): Promise<Skill> => {
  return await api.post('/admin/skills', data) as unknown as Skill;
};

// 更新 Skill
export const updateSkill = async (id: string, data: UpdateSkillRequest): Promise<Skill> => {
  return await api.put(`/admin/skills/${id}`, data) as unknown as Skill;
};

// 删除 Skill
export const deleteSkill = async (id: string, hard?: boolean): Promise<void> => {
  await api.delete(`/admin/skills/${id}`, { params: { hard } });
};

// 激活 Skill
export const activateSkill = async (id: string): Promise<Skill> => {
  return await api.post(`/admin/skills/${id}/activate`) as unknown as Skill;
};

// 停用 Skill
export const deactivateSkill = async (id: string): Promise<Skill> => {
  return await api.post(`/admin/skills/${id}/deactivate`) as unknown as Skill;
};

// 获取 Skill 使用统计
export const getSkillUsageStats = async (id: string): Promise<SkillUsageStats> => {
  return await api.get(`/admin/skills/${id}/usage-stats`) as unknown as SkillUsageStats;
};

// 获取 Module Skill
export const getModuleSkill = async (moduleId: number): Promise<Skill | null> => {
  try {
    const response = await api.get(`/admin/modules/${moduleId}/skill`) as unknown as { skill: Skill };
    return response.skill;
  } catch (error: any) {
    if (error.includes?.('404') || error.status === 404) {
      return null;
    }
    throw error;
  }
};

// 生成 Module Skill
export const generateModuleSkill = async (moduleId: number): Promise<Skill> => {
  const response = await api.post(`/admin/modules/${moduleId}/skill/generate`) as unknown as { skill: Skill };
  return response.skill;
};

// 更新 Module Skill
export const updateModuleSkill = async (moduleId: number, content: string): Promise<Skill> => {
  const response = await api.put(`/admin/modules/${moduleId}/skill`, { content }) as unknown as { skill: Skill };
  return response.skill;
};

// 预览 Module Skill
export const previewModuleSkill = async (moduleId: number): Promise<string> => {
  const response = await api.get(`/admin/modules/${moduleId}/skill/preview`) as unknown as { preview_content: string };
  return response.preview_content;
};

// ========== Module Version Skill API ==========

export interface ModuleVersionSkill {
  id: string;
  module_version_id: string;
  schema_generated_content: string;
  schema_generated_at?: string;
  schema_version_used?: number;
  custom_content: string;
  inherited_from_version_id?: string;
  combined_content: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

// 获取版本的 Skill
export const getModuleVersionSkill = async (versionId: string): Promise<ModuleVersionSkill> => {
  const response = await api.get(`/admin/module-versions/${versionId}/skill`);
  return response as unknown as ModuleVersionSkill;
};

// 根据 Schema 生成 Skill
export const generateModuleVersionSkill = async (versionId: string): Promise<ModuleVersionSkill> => {
  const response = await api.post(`/admin/module-versions/${versionId}/skill/generate`);
  return response as unknown as ModuleVersionSkill;
};

// 更新自定义内容
export const updateModuleVersionSkillCustomContent = async (
  versionId: string,
  customContent: string
): Promise<ModuleVersionSkill> => {
  const response = await api.put(`/admin/module-versions/${versionId}/skill`, {
    custom_content: customContent,
  });
  return response as unknown as ModuleVersionSkill;
};

// 从其他版本继承 Skill
export const inheritModuleVersionSkill = async (
  versionId: string,
  fromVersionId: string
): Promise<ModuleVersionSkill> => {
  const response = await api.post(`/admin/module-versions/${versionId}/skill/inherit`, {
    from_version_id: fromVersionId,
  });
  return response as unknown as ModuleVersionSkill;
};

// 删除版本的 Skill
export const deleteModuleVersionSkill = async (versionId: string): Promise<void> => {
  await api.delete(`/admin/module-versions/${versionId}/skill`);
};

// ========== Domain Skill 自动发现预览 API ==========

export interface DiscoveredSkillSummary {
  name: string;
  display_name: string;
  tags: string[];
  priority: number;
}

export interface PreviewDiscoveryResponse {
  task_skill: string;
  domain_tags: string[];
  discovered_skills: DiscoveredSkillSummary[];
  discovered_count: number;
  message?: string;
}

// 预览 Domain Skill 自动发现结果
export const previewDomainSkillDiscovery = async (taskSkillName: string): Promise<PreviewDiscoveryResponse> => {
  return await api.get('/admin/skills/preview-discovery', {
    params: { task_skill: taskSkillName }
  }) as unknown as PreviewDiscoveryResponse;
};
