// 功能开关配置
export const FEATURES = {
  // 通知系统
  TOAST_NOTIFICATIONS: true,
  
  // 高级表单功能
  ADVANCED_FORMS: false,
  
  // AI解析功能
  AI_PARSING: false,
  
  // 实时更新
  REAL_TIME_UPDATES: false,
  
  // 暗色主题
  DARK_MODE: false,
  
} as const;

// 功能开关类型
export type FeatureFlag = keyof typeof FEATURES;

// 检查功能是否启用
export const isFeatureEnabled = (feature: FeatureFlag): boolean => {
  return FEATURES[feature];
};