import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { adminService, type ProviderTemplate } from '../services/admin';
import type { ProviderInstance } from '../services/workspaces';
import { JsonEditor } from '../components/DynamicForm/JsonEditor';
import api from '../services/api';
import styles from './ProviderSettings.module.css';

type ProviderMode = 'template' | 'custom' | 'none';

interface ProviderSettingsProps {
  workspaceId: string;
}

// 内部用于列表 key，渲染阶段生成；不会发送到后端
type InstanceView = ProviderInstance & { _key: string };

let instanceKeyCounter = 0;
const makeKey = () => `inst-${Date.now()}-${++instanceKeyCounter}`;

const ProviderSettings: React.FC<ProviderSettingsProps> = ({ workspaceId }) => {
  const { showToast } = useToast();

  const [mode, setMode] = useState<ProviderMode>('none');
  const [availableTemplates, setAvailableTemplates] = useState<ProviderTemplate[]>([]);
  const [instances, setInstances] = useState<InstanceView[]>([]);
  const [hasChanges, setHasChanges] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  // Custom mode state
  const [customJson, setCustomJson] = useState('');
  const [jsonError, setJsonError] = useState('');

  // Instance card 折叠状态
  const [expandedInstances, setExpandedInstances] = useState<Set<string>>(new Set());

  // Add Provider dropdown 开关
  const [showAddDropdown, setShowAddDropdown] = useState(false);
  const dropdownRef = useRef<HTMLDivElement | null>(null);

  const fetchConfig = useCallback(async () => {
    try {
      setLoading(true);

      const [workspaceRes, templatesRes] = await Promise.all([
        api.get(`/workspaces/${workspaceId}`),
        adminService.getProviderTemplates({ enabled: true }),
      ]);

      const workspace = workspaceRes.data || workspaceRes;
      const templates = templatesRes.items || [];
      setAvailableTemplates(templates);

      const rawInstances: ProviderInstance[] = Array.isArray(workspace.provider_instances)
        ? workspace.provider_instances
        : [];
      const providerConfig = workspace.provider_config;

      if (rawInstances.length > 0) {
        setMode('template');
        setInstances(
          rawInstances.map((inst) => ({
            template_id: inst.template_id,
            alias: inst.alias ?? '',
            overrides: inst.overrides ?? {},
            _key: makeKey(),
          }))
        );
      } else if (
        providerConfig &&
        typeof providerConfig === 'object' &&
        Object.keys(providerConfig).length > 0
      ) {
        setMode('custom');
        setCustomJson(JSON.stringify(providerConfig, null, 2));
        setInstances([]);
      } else {
        setMode('none');
        setInstances([]);
      }

      setExpandedInstances(new Set());
      setHasChanges(false);
    } catch (error) {
      console.error('Failed to fetch provider config:', error);
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setLoading(false);
    }
  }, [workspaceId, showToast]);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  // 点击外部关闭 dropdown
  useEffect(() => {
    if (!showAddDropdown) return;
    const handler = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setShowAddDropdown(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [showAddDropdown]);

  const handleModeChange = (newMode: ProviderMode) => {
    if (newMode === mode) return;
    setMode(newMode);
    setHasChanges(true);

    if (newMode === 'custom') {
      if (!customJson) {
        setCustomJson('{\n  \n}');
      }
      setJsonError('');
    }
  };

  const handleAddInstance = (template: ProviderTemplate) => {
    const newInst: InstanceView = {
      template_id: template.id,
      alias: '',
      overrides: {},
      _key: makeKey(),
    };
    setInstances((prev) => [...prev, newInst]);
    setExpandedInstances((prev) => new Set([...prev, newInst._key]));
    setShowAddDropdown(false);
    setHasChanges(true);
  };

  const handleRemoveInstance = (key: string) => {
    setInstances((prev) => prev.filter((inst) => inst._key !== key));
    setExpandedInstances((prev) => {
      const next = new Set(prev);
      next.delete(key);
      return next;
    });
    setHasChanges(true);
  };

  const handleAliasChange = (key: string, alias: string) => {
    setInstances((prev) =>
      prev.map((inst) => (inst._key === key ? { ...inst, alias } : inst))
    );
    setHasChanges(true);
  };

  const handleOverrideChange = (key: string, fieldKey: string, value: string) => {
    setInstances((prev) =>
      prev.map((inst) => {
        if (inst._key !== key) return inst;
        const template = availableTemplates.find((t) => t.id === inst.template_id);
        const templateValue = template?.config[fieldKey];
        const defaultStr =
          templateValue != null && typeof templateValue === 'object'
            ? JSON.stringify(templateValue)
            : String(templateValue ?? '');
        const nextOverrides = { ...inst.overrides };
        if (value === defaultStr) {
          delete nextOverrides[fieldKey];
        } else {
          nextOverrides[fieldKey] = parseValue(value);
        }
        return { ...inst, overrides: nextOverrides };
      })
    );
    setHasChanges(true);
  };

  const handleResetOverride = (key: string, fieldKey: string) => {
    setInstances((prev) =>
      prev.map((inst) => {
        if (inst._key !== key) return inst;
        const nextOverrides = { ...inst.overrides };
        delete nextOverrides[fieldKey];
        return { ...inst, overrides: nextOverrides };
      })
    );
    setHasChanges(true);
  };

  const toggleExpanded = (key: string) => {
    setExpandedInstances((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const handleCustomJsonChange = (value: string) => {
    setCustomJson(value);
    setHasChanges(true);
    if (value.trim()) {
      try {
        JSON.parse(value);
        setJsonError('');
      } catch (e: any) {
        setJsonError(e.message || 'Invalid JSON');
      }
    } else {
      setJsonError('');
    }
  };

  // 前端即时校验 alias 唯一性（后端会再校验一次）
  const validateAliases = (): string | null => {
    const typeMap: Record<string, { aliases: Set<string>; defaultCount: number }> = {};
    for (const inst of instances) {
      const tmpl = availableTemplates.find((t) => t.id === inst.template_id);
      if (!tmpl) continue;
      if (!typeMap[tmpl.type]) {
        typeMap[tmpl.type] = { aliases: new Set(), defaultCount: 0 };
      }
      const bucket = typeMap[tmpl.type];
      const alias = inst.alias.trim();
      if (alias === '') {
        bucket.defaultCount++;
        if (bucket.defaultCount > 1) {
          return `provider type "${tmpl.type}" 只允许一个默认实例（无 alias），其余必须设置 alias`;
        }
      } else {
        if (bucket.aliases.has(alias)) {
          return `provider type "${tmpl.type}" 下 alias "${alias}" 重复`;
        }
        bucket.aliases.add(alias);
      }
    }
    return null;
  };

  const handleSave = async () => {
    try {
      setSaving(true);
      let payload: Record<string, any> = {};

      if (mode === 'template') {
        const aliasError = validateAliases();
        if (aliasError) {
          showToast(aliasError, 'error');
          return;
        }
        payload = {
          provider_instances: instances.map(({ _key, ...rest }) => ({
            ...rest,
            alias: rest.alias.trim(),
          })),
        };
      } else if (mode === 'custom') {
        if (!customJson.trim()) {
          showToast('Custom configuration 不能为空；如需清除 provider 配置请切换到 None 模式', 'error');
          return;
        }
        try {
          payload = {
            provider_config: JSON.parse(customJson),
          };
        } catch {
          showToast('Invalid JSON in custom configuration', 'error');
          return;
        }
      } else {
        // None mode: 发空 instances，后端会连带清 provider_config
        payload = {
          provider_instances: [],
        };
      }

      await api.patch(`/workspaces/${workspaceId}`, payload);
      showToast('Provider configuration saved', 'success');
      setHasChanges(false);
      await fetchConfig();
    } catch (error) {
      console.error('Failed to save provider config:', error);
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setSaving(false);
    }
  };

  // Add dropdown 的模板按 type 分组
  const templatesByType = availableTemplates.reduce<Record<string, ProviderTemplate[]>>(
    (acc, template) => {
      const type = template.type || 'other';
      if (!acc[type]) acc[type] = [];
      acc[type].push(template);
      return acc;
    },
    {}
  );

  if (loading) {
    return <div className={styles.loading}>Loading...</div>;
  }

  return (
    <div className={styles.container}>
      {/* Mode Selector */}
      <div className={styles.modeSelector}>
        <label
          className={`${styles.modeOption} ${mode === 'template' ? styles.active : ''}`}
          onClick={() => handleModeChange('template')}
        >
          <input
            type="radio"
            name="providerMode"
            value="template"
            checked={mode === 'template'}
            onChange={() => handleModeChange('template')}
            className={styles.modeRadio}
          />
          <div className={styles.modeContent}>
            <span className={styles.modeTitle}>Use Global Templates</span>
            <span className={styles.modeDescription}>
              Add provider instances from admin-managed templates; same template can be added multiple times with different aliases
            </span>
          </div>
        </label>

        <label
          className={`${styles.modeOption} ${mode === 'custom' ? styles.active : ''}`}
          onClick={() => handleModeChange('custom')}
        >
          <input
            type="radio"
            name="providerMode"
            value="custom"
            checked={mode === 'custom'}
            onChange={() => handleModeChange('custom')}
            className={styles.modeRadio}
          />
          <div className={styles.modeContent}>
            <span className={styles.modeTitle}>Custom Configuration</span>
            <span className={styles.modeDescription}>
              Provide raw provider JSON configuration (legacy)
            </span>
          </div>
        </label>

        <label
          className={`${styles.modeOption} ${mode === 'none' ? styles.active : ''}`}
          onClick={() => handleModeChange('none')}
        >
          <input
            type="radio"
            name="providerMode"
            value="none"
            checked={mode === 'none'}
            onChange={() => handleModeChange('none')}
            className={styles.modeRadio}
          />
          <div className={styles.modeContent}>
            <span className={styles.modeTitle}>None (Module Defaults)</span>
            <span className={styles.modeDescription}>
              Let Terraform use providers from your module code or environment
            </span>
          </div>
        </label>
      </div>

      {(mode === 'template' || mode === 'custom') && (
        <div className={styles.securityNotice}>
          <strong>Security:</strong> 请勿在 Provider 配置中存放密钥、密码等敏感数据。
          敏感凭据应通过 Workspace Variables (Environment Variables) 注入，
          例如 AWS_ACCESS_KEY_ID、AWS_SECRET_ACCESS_KEY。
        </div>
      )}

      {/* Template Mode: Instance list */}
      {mode === 'template' && (
        <div className={styles.templateSection}>
          {availableTemplates.length === 0 ? (
            <div className={styles.infoBox}>
              No enabled provider templates available. Ask your administrator to create provider templates in Global Settings.
            </div>
          ) : (
            <>
              <div className={styles.addProviderRow} ref={dropdownRef}>
                <button
                  type="button"
                  className={styles.addProviderButton}
                  onClick={() => setShowAddDropdown((v) => !v)}
                >
                  + Add Provider
                </button>
                {showAddDropdown && (
                  <div className={styles.addProviderDropdown}>
                    {Object.keys(templatesByType).length === 0 ? (
                      <div className={styles.addProviderEmpty}>No templates available</div>
                    ) : (
                      Object.entries(templatesByType).map(([type, tmpls]) => (
                        <div key={type} className={styles.addProviderDropdownGroup}>
                          <div className={styles.addProviderDropdownGroupTitle}>
                            {type.toUpperCase()}
                          </div>
                          {tmpls.map((tmpl) => (
                            <button
                              key={tmpl.id}
                              type="button"
                              className={styles.addProviderDropdownItem}
                              onClick={() => handleAddInstance(tmpl)}
                            >
                              <span>
                                {tmpl.name}
                                {tmpl.is_default && (
                                  <span className={styles.defaultBadge} style={{ marginLeft: 8 }}>
                                    Default
                                  </span>
                                )}
                              </span>
                              <span className={styles.addProviderDropdownItemMeta}>
                                {tmpl.source}
                                {tmpl.version &&
                                  ` ${tmpl.constraint_op || '~>'} ${tmpl.version}`}
                              </span>
                            </button>
                          ))}
                        </div>
                      ))
                    )}
                  </div>
                )}
              </div>

              {instances.length === 0 ? (
                <div className={styles.infoBox}>
                  <div className={styles.infoIcon}>i</div>
                  <div className={styles.infoContent}>
                    <strong>No provider instances configured</strong>
                    <p>
                      Click "Add Provider" to add an instance from a global template. Same
                      template can be added multiple times with different aliases.
                    </p>
                  </div>
                </div>
              ) : (
                <div className={styles.instanceList}>
                  {instances.map((inst) => {
                    const template = availableTemplates.find((t) => t.id === inst.template_id);
                    const configKeys = Object.keys(template?.config || {});
                    const isExpanded = expandedInstances.has(inst._key);

                    return (
                      <div key={inst._key} className={styles.instanceCard}>
                        <div className={styles.instanceHeader}>
                          <div className={styles.instanceTitle}>
                            <span>{template?.name ?? `(模板 #${inst.template_id} 已删除)`}</span>
                            {template && (
                              <span className={styles.typeBadge}>{template.type}</span>
                            )}
                            {inst.alias && (
                              <span className={styles.defaultBadge}>alias: {inst.alias}</span>
                            )}
                          </div>
                          <div className={styles.aliasField}>
                            <label className={styles.aliasLabel}>alias:</label>
                            <input
                              type="text"
                              className={styles.aliasInput}
                              value={inst.alias}
                              onChange={(e) => handleAliasChange(inst._key, e.target.value)}
                              placeholder="默认实例（留空）"
                            />
                          </div>
                          {configKeys.length > 0 && (
                            <button
                              type="button"
                              className={styles.expandButton}
                              onClick={() => toggleExpanded(inst._key)}
                            >
                              {isExpanded ? 'Hide Overrides' : 'Show Overrides'}
                            </button>
                          )}
                          <button
                            type="button"
                            className={styles.removeInstanceButton}
                            onClick={() => handleRemoveInstance(inst._key)}
                          >
                            Remove
                          </button>
                        </div>

                        {template && isExpanded && configKeys.length > 0 && (
                          <div className={styles.overrideSection}>
                            <div className={styles.overrideSectionHeader}>
                              <span className={styles.overrideSectionTitle}>
                                Configuration Overrides
                              </span>
                              <span className={styles.overrideHint}>
                                Modify values to override template defaults
                              </span>
                            </div>
                            {configKeys.map((key) => {
                              const templateValue = template.config[key];
                              const isOverridden = key in inst.overrides;
                              const rawValue = isOverridden ? inst.overrides[key] : templateValue;
                              const displayValue =
                                rawValue != null && typeof rawValue === 'object'
                                  ? JSON.stringify(rawValue)
                                  : String(rawValue ?? '');
                              const placeholderValue =
                                templateValue != null && typeof templateValue === 'object'
                                  ? JSON.stringify(templateValue)
                                  : String(templateValue ?? '');

                              return (
                                <div key={key} className={styles.overrideField}>
                                  <label
                                    className={`${styles.overrideLabel} ${isOverridden ? styles.overridden : ''}`}
                                  >
                                    {key}
                                    {isOverridden && (
                                      <span className={styles.overriddenIndicator}>
                                        (overridden)
                                      </span>
                                    )}
                                  </label>
                                  <input
                                    type="text"
                                    className={`${styles.overrideInput} ${isOverridden ? styles.overriddenInput : ''}`}
                                    value={displayValue}
                                    onChange={(e) =>
                                      handleOverrideChange(inst._key, key, e.target.value)
                                    }
                                    placeholder={placeholderValue}
                                  />
                                  {isOverridden && (
                                    <button
                                      type="button"
                                      className={styles.resetButton}
                                      onClick={() => handleResetOverride(inst._key, key)}
                                      title="Reset to template default"
                                    >
                                      Reset
                                    </button>
                                  )}
                                </div>
                              );
                            })}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* Custom Mode UI */}
      {mode === 'custom' && (
        <div className={styles.customSection}>
          <div className={styles.customHeader}>
            <span className={styles.customTitle}>Provider Configuration JSON</span>
            <span className={styles.customHint}>
              Paste or edit raw provider_config JSON. This will be used to generate provider.tf.json.
            </span>
          </div>
          <JsonEditor
            value={customJson}
            onChange={handleCustomJsonChange}
            placeholder='{"provider": {"aws": [{"region": "us-east-1"}]}}'
            minHeight={250}
            maxHeight={600}
          />
        </div>
      )}

      {/* None Mode UI */}
      {mode === 'none' && (
        <div className={styles.infoBox}>
          <div className={styles.infoIcon}>i</div>
          <div className={styles.infoContent}>
            <strong>No provider configuration</strong>
            <p>
              Terraform will use provider settings from your module code or environment variables.
              This is suitable when providers are configured directly in your .tf files or via
              environment-based authentication (e.g., AWS_PROFILE, GOOGLE_CREDENTIALS).
            </p>
          </div>
        </div>
      )}

      <div className={styles.actions}>
        <button
          onClick={handleSave}
          className={styles.saveButton}
          disabled={!hasChanges || saving || (mode === 'custom' && !!jsonError)}
        >
          {saving ? 'Saving...' : 'Save Settings'}
        </button>
        {hasChanges && <span className={styles.unsavedHint}>You have unsaved changes</span>}
      </div>
    </div>
  );
};

/** Try to parse a string value into its appropriate JS type */
function parseValue(value: string): any {
  if (value === 'true') return true;
  if (value === 'false') return false;
  if (/^\d+$/.test(value)) return parseInt(value, 10);
  if (/^\d+\.\d+$/.test(value)) return parseFloat(value);
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}

export default ProviderSettings;
