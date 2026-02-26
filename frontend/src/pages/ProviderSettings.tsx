import React, { useState, useEffect, useCallback } from 'react';
import { useToast } from '../contexts/ToastContext';
import { extractErrorMessage } from '../utils/errorHandler';
import { adminService, type ProviderTemplate } from '../services/admin';
import { JsonEditor } from '../components/DynamicForm/JsonEditor';
import api from '../services/api';
import styles from './ProviderSettings.module.css';

type ProviderMode = 'template' | 'custom' | 'none';

interface ProviderSettingsProps {
  workspaceId: string;
}

const ProviderSettings: React.FC<ProviderSettingsProps> = ({ workspaceId }) => {
  const { showToast } = useToast();

  const [mode, setMode] = useState<ProviderMode>('none');
  const [availableTemplates, setAvailableTemplates] = useState<ProviderTemplate[]>([]);
  const [selectedTemplateIds, setSelectedTemplateIds] = useState<number[]>([]);
  const [overrides, setOverrides] = useState<Record<string, Record<string, any>>>({});
  const [hasChanges, setHasChanges] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  // Custom mode state
  const [customJson, setCustomJson] = useState('');
  const [jsonError, setJsonError] = useState('');

  // Collapsible override sections
  const [expandedTemplates, setExpandedTemplates] = useState<Set<number>>(new Set());

  const fetchConfig = useCallback(async () => {
    try {
      setLoading(true);

      // Fetch workspace data and available templates in parallel
      const [workspaceRes, templatesRes] = await Promise.all([
        api.get(`/workspaces/${workspaceId}`),
        adminService.getProviderTemplates({ enabled: true }),
      ]);

      const workspace = workspaceRes.data || workspaceRes;
      const templates = templatesRes.items || [];
      setAvailableTemplates(templates);

      // Determine initial mode
      const templateIds = workspace.provider_template_ids;
      const providerConfig = workspace.provider_config;
      const providerOverrides = workspace.provider_overrides;

      if (Array.isArray(templateIds) && templateIds.length > 0) {
        // Template mode
        setMode('template');
        setSelectedTemplateIds(templateIds);
        setOverrides(providerOverrides || {});
        setExpandedTemplates(new Set());
      } else if (
        providerConfig &&
        typeof providerConfig === 'object' &&
        Object.keys(providerConfig).length > 0
      ) {
        // Custom mode
        setMode('custom');
        setCustomJson(JSON.stringify(providerConfig, null, 2));
      } else {
        // None mode
        setMode('none');
      }

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

  // Mode change handler
  const handleModeChange = (newMode: ProviderMode) => {
    if (newMode === mode) return;
    setMode(newMode);
    setHasChanges(true);

    if (newMode === 'template') {
      // Keep existing selections if any
    } else if (newMode === 'custom') {
      if (!customJson) {
        setCustomJson('{\n  \n}');
      }
      setJsonError('');
    }
  };

  // Template selection handler
  const handleTemplateToggle = (templateId: number) => {
    setSelectedTemplateIds((prev) => {
      if (prev.includes(templateId)) {
        // Remove the template and its overrides
        const newOverrides = { ...overrides };
        delete newOverrides[String(templateId)];
        setOverrides(newOverrides);
        return prev.filter((id) => id !== templateId);
      } else {
        // Add the template
        setExpandedTemplates((exp) => new Set([...exp, templateId]));
        return [...prev, templateId];
      }
    });
    setHasChanges(true);
  };

  // Override change handler
  const handleOverrideChange = (templateId: number, key: string, value: string) => {
    const template = availableTemplates.find((t) => t.id === templateId);
    if (!template) return;

    const tidStr = String(templateId);

    setOverrides((prev) => {
      const templateOverrides = { ...(prev[tidStr] || {}) };

      // Alias is always stored as a string and kept in overrides even when empty
      // (empty string tells backend to clear any legacy template-level alias)
      if (key === 'alias') {
        templateOverrides[key] = value;
      } else {
        const templateValue = template.config[key];
        // If value matches template default, remove the override
        const defaultStr =
          templateValue != null && typeof templateValue === 'object'
            ? JSON.stringify(templateValue)
            : String(templateValue ?? '');
        if (value === defaultStr) {
          delete templateOverrides[key];
        } else {
          // Try to parse as JSON/number/boolean for storage
          templateOverrides[key] = parseValue(value);
        }
      }

      const newOverrides = { ...prev };
      if (Object.keys(templateOverrides).length === 0) {
        delete newOverrides[tidStr];
      } else {
        newOverrides[tidStr] = templateOverrides;
      }
      return newOverrides;
    });
    setHasChanges(true);
  };

  // Reset a single override to template default
  const handleResetOverride = (templateId: number, key: string) => {
    const tidStr = String(templateId);
    setOverrides((prev) => {
      const templateOverrides = { ...(prev[tidStr] || {}) };
      delete templateOverrides[key];

      const newOverrides = { ...prev };
      if (Object.keys(templateOverrides).length === 0) {
        delete newOverrides[tidStr];
      } else {
        newOverrides[tidStr] = templateOverrides;
      }
      return newOverrides;
    });
    setHasChanges(true);
  };

  // Toggle expand/collapse for a template override section
  const toggleExpanded = (templateId: number) => {
    setExpandedTemplates((prev) => {
      const next = new Set(prev);
      if (next.has(templateId)) {
        next.delete(templateId);
      } else {
        next.add(templateId);
      }
      return next;
    });
  };

  // Custom JSON change handler
  const handleCustomJsonChange = (value: string) => {
    setCustomJson(value);
    setHasChanges(true);

    // Validate JSON
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

  // Save handler
  const handleSave = async () => {
    try {
      setSaving(true);
      let payload: Record<string, any> = {};

      if (mode === 'template') {
        // Validate alias uniqueness per provider type
        const aliasErrors: string[] = [];
        const typeAliasMap: Record<string, { aliases: string[]; defaultCount: number }> = {};

        selectedTemplateIds.forEach(id => {
          const tmpl = availableTemplates.find(t => t.id === id);
          if (!tmpl) return;
          if (!typeAliasMap[tmpl.type]) {
            typeAliasMap[tmpl.type] = { aliases: [], defaultCount: 0 };
          }
          const alias = overrides[String(id)]?.alias ?? '';
          if (alias) {
            if (typeAliasMap[tmpl.type].aliases.includes(alias)) {
              aliasErrors.push(`${tmpl.type}: alias "${alias}" 重复`);
            }
            typeAliasMap[tmpl.type].aliases.push(alias);
          } else {
            typeAliasMap[tmpl.type].defaultCount++;
            if (typeAliasMap[tmpl.type].defaultCount > 1) {
              aliasErrors.push(`${tmpl.type}: 只允许一个默认 Provider（无 alias），其余必须设置 alias`);
            }
          }
        });

        if (aliasErrors.length > 0) {
          showToast(aliasErrors.join('; '), 'error');
          return;
        }

        payload = {
          provider_template_ids: selectedTemplateIds,
          provider_overrides: Object.keys(overrides).length > 0 ? overrides : null,
          provider_config: null,
        };
      } else if (mode === 'custom') {
        // Validate JSON before saving
        if (customJson.trim()) {
          try {
            const parsed = JSON.parse(customJson);
            payload = {
              provider_config: parsed,
              provider_template_ids: [],
              provider_overrides: null,
            };
          } catch {
            showToast('Invalid JSON in custom configuration', 'error');
            return;
          }
        } else {
          payload = {
            provider_config: null,
            provider_template_ids: [],
            provider_overrides: null,
          };
        }
      } else {
        // None mode
        payload = {
          provider_template_ids: [],
          provider_config: null,
          provider_overrides: null,
        };
      }

      await api.patch(`/workspaces/${workspaceId}`, payload);
      showToast('Provider configuration saved', 'success');
      setHasChanges(false);

      // Reload to confirm
      await fetchConfig();
    } catch (error) {
      console.error('Failed to save provider config:', error);
      showToast(extractErrorMessage(error), 'error');
    } finally {
      setSaving(false);
    }
  };

  // Group templates by type
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
              Select from admin-managed provider templates with optional overrides
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

      {/* Security Notice */}
      {(mode === 'template' || mode === 'custom') && (
        <div className={styles.securityNotice}>
          <strong>Security:</strong> 请勿在 Provider 配置中存放密钥、密码等敏感数据。
          敏感凭据应通过 Workspace Variables (Environment Variables) 注入，
          例如 AWS_ACCESS_KEY_ID、AWS_SECRET_ACCESS_KEY。
        </div>
      )}

      {/* Template Mode UI */}
      {mode === 'template' && (
        <div className={styles.templateSection}>
          {Object.keys(templatesByType).length === 0 ? (
            <div className={styles.infoBox}>
              No enabled provider templates available. Ask your administrator to create provider
              templates in Global Settings.
            </div>
          ) : (
            Object.entries(templatesByType).map(([type, templates]) => (
              <div key={type} className={styles.templateGroup}>
                <h4 className={styles.templateGroupTitle}>{type.toUpperCase()}</h4>
                <div className={styles.templateList}>
                  {templates.map((template) => {
                    const isSelected = selectedTemplateIds.includes(template.id);
                    const isExpanded = expandedTemplates.has(template.id);
                    const tidStr = String(template.id);
                    const templateOverrides = overrides[tidStr] || {};
                    const configKeys = Object.keys(template.config || {});

                    return (
                      <div
                        key={template.id}
                        className={`${styles.templateCard} ${isSelected ? styles.selected : ''}`}
                      >
                        <div className={styles.templateHeader}>
                          <label className={styles.templateCheckbox}>
                            <input
                              type="checkbox"
                              checked={isSelected}
                              onChange={() => handleTemplateToggle(template.id)}
                            />
                            <div className={styles.templateInfo}>
                              <span className={styles.templateName}>
                                {template.name}
                                {isSelected && templateOverrides.alias && (
                                  <span className={styles.defaultBadge}>alias: {templateOverrides.alias}</span>
                                )}
                                {template.is_default && (
                                  <span className={styles.defaultBadge}>Default</span>
                                )}
                              </span>
                              <span className={styles.templateMeta}>
                                {template.source}
                                {template.version &&
                                  ` ${template.constraint_op || '~>'} ${template.version}`}
                              </span>
                              {template.description && (
                                <span className={styles.templateDescription}>
                                  {template.description}
                                </span>
                              )}
                            </div>
                          </label>
                          {isSelected && (
                            <div className={styles.aliasField}>
                              <label className={styles.aliasLabel}>alias:</label>
                              <input
                                type="text"
                                className={styles.aliasInput}
                                value={templateOverrides.alias ?? ''}
                                onChange={(e) => handleOverrideChange(template.id, 'alias', e.target.value)}
                                placeholder="默认 Provider（无需 alias）"
                              />
                            </div>
                          )}
                          {isSelected && configKeys.length > 0 && (
                            <button
                              type="button"
                              className={styles.expandButton}
                              onClick={() => toggleExpanded(template.id)}
                            >
                              {isExpanded ? 'Hide Overrides' : 'Show Overrides'}
                            </button>
                          )}
                        </div>

                        {/* Override Section */}
                        {isSelected && isExpanded && configKeys.length > 0 && (
                          <div className={styles.overrideSection}>
                            <div className={styles.overrideSectionHeader}>
                              <span className={styles.overrideSectionTitle}>
                                Configuration Overrides
                              </span>
                              <span className={styles.overrideHint}>
                                Modify values to override template defaults
                              </span>
                            </div>
                            {configKeys.filter(key => key !== 'alias').map((key) => {
                              const templateValue = template.config[key];
                              const isOverridden = key in templateOverrides;
                              const rawValue = isOverridden
                                ? templateOverrides[key]
                                : templateValue;
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
                                      handleOverrideChange(template.id, key, e.target.value)
                                    }
                                    placeholder={placeholderValue}
                                  />
                                  {isOverridden && (
                                    <button
                                      type="button"
                                      className={styles.resetButton}
                                      onClick={() => handleResetOverride(template.id, key)}
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
              </div>
            ))
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

      {/* Save Actions */}
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
