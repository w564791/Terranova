import React, { useMemo, useCallback, useEffect, useRef, useState } from 'react';
import { Form, Collapse, Empty, Tooltip, Tabs, Row, Col } from 'antd';
import { InfoCircleOutlined } from '@ant-design/icons';
import type { FormRendererProps, FieldRenderConfig, WidgetType, GroupConfig, OpenAPIFormSchema, CascadeRule } from './types';
import { getWidget } from './widgets';
import { getWidgetType, type PropertySchema, type FieldUIConfig } from '../../services/schemaV2';
import { CascadeEngine, type CascadeState } from './CascadeEngine';
import { AIConfigGenerator } from './AIFormAssistant';
import styles from './FormRenderer.module.css';

const { Panel } = Collapse;
const { TabPane } = Tabs;

/**
 * 从 Schema 中提取默认值
 * 关键：使用 hasOwnProperty 检查，确保 false, 0, "", [], {} 等 falsy 值也能被正确提取
 */
const extractDefaultValues = (schema: OpenAPIFormSchema): Record<string, unknown> => {
  const properties = schema.components?.schemas?.ModuleInput?.properties || {};
  const defaults: Record<string, unknown> = {};
  
  Object.entries(properties).forEach(([name, prop]) => {
    const property = prop as PropertySchema;
    
    // 使用 hasOwnProperty 检查，而不是检查值是否为 truthy
    if (Object.prototype.hasOwnProperty.call(property, 'default')) {
      const defaultValue = property.default;
      const propType = property.type;
      
      // 根据类型进行转换，确保 falsy 值也能正确处理
      switch (propType) {
        case 'boolean':
          // boolean 类型：false 是有效值
          defaults[name] = defaultValue === true;
          break;
        case 'integer':
          // integer 类型：0 是有效值
          defaults[name] = defaultValue !== null && defaultValue !== undefined 
            ? Number(defaultValue) 
            : 0;
          break;
        case 'number':
          // number 类型：0.0 是有效值
          defaults[name] = defaultValue !== null && defaultValue !== undefined 
            ? parseFloat(String(defaultValue)) 
            : 0;
          break;
        case 'string':
          // string 类型：空字符串是有效值
          defaults[name] = defaultValue ?? '';
          break;
        case 'array':
          // array 类型：空数组是有效值
          defaults[name] = Array.isArray(defaultValue) ? defaultValue : [];
          break;
        case 'object':
          // object 类型：空对象是有效值
          if (property.properties) {
            // 有 properties 定义的对象，递归提取嵌套默认值
            defaults[name] = extractNestedDefaults(property, defaultValue);
          } else {
            // additionalProperties 类型的对象（如 map）
            defaults[name] = (defaultValue && typeof defaultValue === 'object') ? defaultValue : {};
          }
          break;
        default:
          defaults[name] = defaultValue;
      }
    }
  });
  
  return defaults;
};

/**
 * 递归提取嵌套对象的默认值
 */
const extractNestedDefaults = (prop: PropertySchema, parentDefault: unknown): Record<string, unknown> => {
  const result: Record<string, unknown> = {};
  const properties = prop.properties || {};
  const defaultObj = (parentDefault && typeof parentDefault === 'object' && !Array.isArray(parentDefault)) 
    ? parentDefault as Record<string, unknown>
    : {};
  
  Object.entries(properties).forEach(([key, nestedProp]) => {
    const nested = nestedProp as PropertySchema;
    if (Object.prototype.hasOwnProperty.call(nested, 'default')) {
      result[key] = nested.default;
    } else if (Object.prototype.hasOwnProperty.call(defaultObj, key)) {
      result[key] = defaultObj[key];
    }
  });
  
  return result;
};

const FormRenderer: React.FC<FormRendererProps> = ({
  schema,
  initialValues = {},
  onChange,
  onSubmit,
  disabled = false,
  readOnly = false,
  providers,
  workspace,
  organization,
  manifest,
  workspaceResource,
  activeGroupId,
  onGroupChange,
  aiAssistant,
}) => {
  const [form] = Form.useForm();
  
  // 级联状态
  const [cascadeState, setCascadeState] = useState<CascadeState>({
    visibility: {},
    disabled: {},
    disabledReasons: {},
    required: {},
    pendingValues: {},
  });
  
  // 内部 Tab 状态（用于非受控模式）
  const [internalActiveGroupId, setInternalActiveGroupId] = useState<string | undefined>(undefined);
  
  // 级联引擎实例
  const cascadeEngineRef = useRef<CascadeEngine | null>(null);

  // 保存原始 initialValues 的引用，用于在 onChange 时合并
  const originalInitialValuesRef = useRef<Record<string, unknown>>(initialValues);

  // 追踪用户主动操作过的字段（编辑、cascade setValue 等）
  const touchedFieldsRef = useRef<Set<string>>(new Set(Object.keys(initialValues)));

  // 更新原始值引用（仅在 initialValues 变化时）
  useEffect(() => {
    originalInitialValuesRef.current = initialValues;
    touchedFieldsRef.current = new Set(Object.keys(initialValues));
  }, [initialValues]);

  // 从 Schema 中提取默认值，并与 initialValues 合并
  // initialValues 优先级更高，会覆盖 Schema 中的默认值
  const mergedInitialValues = useMemo(() => {
    const schemaDefaults = extractDefaultValues(schema);
    const merged = { ...schemaDefaults, ...initialValues };
    return merged;
  }, [schema, initialValues]);

  // 提取标记了 x-renderDefault 的字段名
  const renderDefaultFieldsRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    const properties = schema.components?.schemas?.ModuleInput?.properties || {};
    const fields = new Set<string>();
    Object.entries(properties).forEach(([name, prop]) => {
      const p = prop as PropertySchema;
      if (p['x-renderDefault'] === true && Object.prototype.hasOwnProperty.call(p, 'default')) {
        fields.add(name);
      }
    });
    renderDefaultFieldsRef.current = fields;
  }, [schema]);

  // 获取级联规则（合并全局规则和字段级配置）
  const cascadeRules = useMemo((): CascadeRule[] => {
    const globalRules = schema['x-iac-platform']?.cascade?.rules || [];
    const uiFields = schema['x-iac-platform']?.ui?.fields || {};
    
    // 从字段级 cascade 配置生成规则
    const fieldRules: CascadeRule[] = [];
    
    Object.entries(uiFields).forEach(([fieldName, uiConfig]) => {
      const cascade = (uiConfig as any).cascade;
      if (!cascade) return;
      
      // showWhen: 当条件满足时显示字段
      if (cascade.showWhen?.field) {
        fieldRules.push({
          id: `${fieldName}-show`,
          description: `显示 ${fieldName}`,
          trigger: {
            field: cascade.showWhen.field,
            operator: cascade.showWhen.operator || 'eq',
            value: cascade.showWhen.value,
          },
          actions: [{ type: 'show', fields: [fieldName] }],
        });
        
        // 同时添加反向规则：条件不满足时隐藏
        const reverseOperator = cascade.showWhen.operator === 'eq' ? 'ne' : 
                               cascade.showWhen.operator === 'ne' ? 'eq' :
                               cascade.showWhen.operator === 'empty' ? 'notEmpty' :
                               cascade.showWhen.operator === 'notEmpty' ? 'empty' : 'ne';
        fieldRules.push({
          id: `${fieldName}-hide-reverse`,
          description: `隐藏 ${fieldName}（反向规则）`,
          trigger: {
            field: cascade.showWhen.field,
            operator: reverseOperator as any,
            value: cascade.showWhen.value,
          },
          actions: [{ type: 'hide', fields: [fieldName] }],
        });
      }
      
      // hideWhen: 当条件满足时隐藏字段
      if (cascade.hideWhen?.field) {
        fieldRules.push({
          id: `${fieldName}-hide`,
          description: `隐藏 ${fieldName}`,
          trigger: {
            field: cascade.hideWhen.field,
            operator: cascade.hideWhen.operator || 'eq',
            value: cascade.hideWhen.value,
          },
          actions: [{ type: 'hide', fields: [fieldName] }],
        });
      }
      
      // requiredWith: 当此字段有值时，依赖字段也必须有值
      if (cascade.requiredWith?.length > 0) {
        fieldRules.push({
          id: `${fieldName}-required-with`,
          description: `${fieldName} 依赖字段`,
          trigger: {
            field: fieldName,
            operator: 'notEmpty',
          },
          actions: [{ type: 'setRequired', fields: cascade.requiredWith, required: true }],
        });
      }
      
      // conflictsWith: 当此字段有值时，冲突字段被清空
      if (cascade.conflictsWith?.length > 0) {
        fieldRules.push({
          id: `${fieldName}-conflicts-with`,
          description: `${fieldName} 冲突字段`,
          trigger: {
            field: fieldName,
            operator: 'notEmpty',
          },
          actions: cascade.conflictsWith.map((f: string) => ({ type: 'clearValue' as const, field: f })),
        });
      }
    });
    
    // 合并全局规则和字段级规则
    return [...globalRules, ...fieldRules];
  }, [schema]);

  // 初始化级联引擎
  useEffect(() => {
    if (cascadeRules.length > 0) {
      cascadeEngineRef.current = new CascadeEngine(cascadeRules);
      // 初始评估
      const initialState = cascadeEngineRef.current.evaluate(mergedInitialValues);
      setCascadeState(initialState);
    }
  }, [cascadeRules, mergedInitialValues]);

  // 当 mergedInitialValues 变化时，更新表单值
  useEffect(() => {
    form.setFieldsValue(mergedInitialValues);
  }, [form, mergedInitialValues]);

  // 解析 Schema 获取字段配置
  const fieldConfigs = useMemo((): FieldRenderConfig[] => {
    const properties = schema.components?.schemas?.ModuleInput?.properties || {};
    const required = schema.components?.schemas?.ModuleInput?.required || [];
    const uiFields = schema['x-iac-platform']?.ui?.fields || {};
    
    return Object.entries(properties).map(([name, property]) => {
      const uiConfig = uiFields[name] || {};
      const widget = getWidgetType(property as PropertySchema, uiConfig as FieldUIConfig) as WidgetType;
      
      return {
        name,
        schema: property as PropertySchema,
        uiConfig: uiConfig as FieldUIConfig,
        widget,
        required: required.includes(name),
        group: uiConfig.group || 'advanced',
        order: uiConfig.order || 999,
        visible: true,
        disabled: disabled || uiConfig.readonly === true,
      };
    }).sort((a, b) => a.order - b.order);
  }, [schema, disabled]);

  // 获取分组配置
  const groups = useMemo((): GroupConfig[] => {
    const schemaGroups = schema['x-iac-platform']?.ui?.groups;
    if (schemaGroups && Array.isArray(schemaGroups) && schemaGroups.length > 0) {
      // 转换 Schema 中的分组格式
      return schemaGroups.map((g: any) => ({
        id: g.id,
        title: g.label || g.title || g.id,
        order: g.order || 100,
        defaultExpanded: g.level === 'basic',
        layout: g.layout || 'sections',
        level: g.level || 'advanced',
      })).sort((a: GroupConfig, b: GroupConfig) => (a.order || 0) - (b.order || 0));
    }
    
    const defaultGroups: GroupConfig[] = [
      { id: 'basic', title: '基础配置', order: 1, defaultExpanded: true, layout: 'sections', level: 'basic' },
      { id: 'advanced', title: '高级配置', order: 2, defaultExpanded: false, layout: 'accordion', level: 'advanced' },
    ];
    return defaultGroups;
  }, [schema]);

  // 获取全局布局模式
  // 优先级：如果任何分组使用 tabs，则整体使用 tabs 布局
  // 否则如果所有分组使用相同的 layout，返回该 layout
  // 否则使用混合模式
  const globalLayout = useMemo(() => {
    if (groups.length === 0) return 'sections';
    
    const layouts = groups.map(g => g.layout || 'sections');
    
    // 如果任何分组使用 tabs，则整体使用 tabs 布局
    // 因为 tabs 布局需要将所有分组合并到一个标签页组件中
    if (layouts.includes('tabs')) {
      return 'tabs';
    }
    
    // 如果所有分组使用相同的 layout，返回该 layout
    const uniqueLayouts = new Set(layouts);
    if (uniqueLayouts.size === 1) return layouts[0];
    
    // 否则使用混合模式
    return 'mixed';
  }, [groups]);

  // 按分组组织字段（应用级联可见性）
  const groupedFields = useMemo(() => {
    return groups.map(group => ({
      ...group,
      fields: fieldConfigs.filter(f => {
        // 检查字段是否在该分组
        if (f.group !== group.id) return false;
        
        // 检查级联可见性
        const cascadeVisible = cascadeState.visibility[f.name];
        // 如果级联规则明确设置了可见性，使用级联规则的值
        // 否则使用字段配置的默认可见性
        const isVisible = cascadeVisible !== undefined ? cascadeVisible : f.visible;
        
        return isVisible;
      }),
    })).filter(g => g.fields.length > 0);
  }, [groups, fieldConfigs, cascadeState.visibility]);

  // 使用 ref 保存最新的 cascadeState，避免闭包问题
  const cascadeStateRef = useRef<CascadeState>(cascadeState);
  useEffect(() => {
    cascadeStateRef.current = cascadeState;
  }, [cascadeState]);

  // 处理表单值变化
  const handleValuesChange = useCallback((_changedValues: Record<string, unknown>, _allValues: Record<string, unknown>) => {
    // 关键修复：使用 form.getFieldsValue(true) 获取所有字段的值
    // 而不是使用 onValuesChange 回调中的 allValues
    // 因为 allValues 只包含当前渲染的字段，隐藏的字段不会出现在其中
    const formValues = form.getFieldsValue(true);
    
    // 评估级联规则
    let currentCascadeState = cascadeStateRef.current;
    if (cascadeEngineRef.current) {
      const newState = cascadeEngineRef.current.evaluate(formValues);
      setCascadeState(newState);
      currentCascadeState = newState;
      cascadeStateRef.current = newState;
      
      // 处理 pendingValues（由 setValue 动作设置的值）
      const pendingValues = newState.pendingValues;
      if (Object.keys(pendingValues).length > 0) {
        // 过滤掉 undefined 值（clearValue 动作）
        const valuesToSet: Record<string, unknown> = {};
        const valuesToClear: string[] = [];
        
        Object.entries(pendingValues).forEach(([key, value]) => {
          if (value === undefined) {
            valuesToClear.push(key);
          } else {
            valuesToSet[key] = value;
          }
        });
        
        // 设置新值
        if (Object.keys(valuesToSet).length > 0) {
          form.setFieldsValue(valuesToSet);
        }
        
        // 清空值
        if (valuesToClear.length > 0) {
          const clearValues: Record<string, undefined> = {};
          valuesToClear.forEach(key => {
            clearValues[key] = undefined;
          });
          form.setFieldsValue(clearValues);
        }
      }
    }
    
    // 记录用户主动触碰的字段
    Object.keys(_changedValues).forEach(key => {
      touchedFieldsRef.current.add(key);
    });

    // 关键修复：合并原始数据和表单数据
    // 原始数据中可能包含不在 schema 中定义的字段，这些字段不会被表单渲染
    // 但在提交时需要保留这些字段，否则会导致数据丢失
    // 合并策略：原始数据作为基础，表单数据覆盖（表单数据优先级更高）
    const mergedValues = {
      ...originalInitialValuesRef.current,  // 原始数据（包含不在 schema 中的字段）
      ...formValues,                         // 表单数据（覆盖原始数据中的同名字段）
    };

    // 过滤掉值为 undefined 的字段（用户明确清空的字段）
    // 同时过滤掉被级联规则**明确**隐藏的字段
    // 只输出：原始数据字段 + 用户触碰的字段 + 标记了 x-renderDefault 的字段
    const filteredValues: Record<string, unknown> = {};
    Object.entries(mergedValues).forEach(([key, value]) => {
      if (value !== undefined) {
        const isExplicitlyHidden = currentCascadeState.visibility[key] === false;
        if (!isExplicitlyHidden) {
          const isOriginalData = key in originalInitialValuesRef.current;
          const isTouched = touchedFieldsRef.current.has(key);
          const isRenderDefault = renderDefaultFieldsRef.current.has(key);
          if (isOriginalData || isTouched || isRenderDefault) {
            filteredValues[key] = value;
          }
        }
      }
    });

    onChange?.(filteredValues);
  }, [onChange, form]);

  // 处理表单提交
  const handleFinish = useCallback((values: Record<string, unknown>) => {
    onSubmit?.(values);
  }, [onSubmit]);

  // 处理 AI 生成的配置
  const handleAIGenerate = useCallback((config: Record<string, unknown>) => {
    // 合并 AI 生成的配置到表单
    const currentValues = form.getFieldsValue(true);
    const mergedValues = { ...currentValues, ...config };
    form.setFieldsValue(mergedValues);
    onChange?.(mergedValues);
  }, [form, onChange]);

  // 手动触发值更新的回调（用于 Widget 在 setFieldsValue 后通知 FormRenderer）
  const triggerValuesUpdate = useCallback(() => {
    const formValues = form.getFieldsValue(true);

    // 合并原始数据和表单数据
    const mergedValues = {
      ...originalInitialValuesRef.current,
      ...formValues,
    };

    // 过滤掉值为 undefined 的字段
    // 只输出：原始数据字段 + 用户触碰的字段 + 标记了 x-renderDefault 的字段
    const filteredValues: Record<string, unknown> = {};
    Object.entries(mergedValues).forEach(([key, value]) => {
      if (value !== undefined) {
        const isExplicitlyHidden = cascadeStateRef.current.visibility[key] === false;
        if (!isExplicitlyHidden) {
          const isOriginalData = key in originalInitialValuesRef.current;
          const isTouched = touchedFieldsRef.current.has(key);
          const isRenderDefault = renderDefaultFieldsRef.current.has(key);
          if (isOriginalData || isTouched || isRenderDefault) {
            filteredValues[key] = value;
          }
        }
      }
    });

    onChange?.(filteredValues);
  }, [onChange, form]);

  // 渲染单个字段
  const renderField = (config: FieldRenderConfig) => {
    const Widget = getWidget(config.widget);
    
    // 检查级联禁用状态
    const cascadeDisabled = cascadeState.disabled[config.name] ?? false;
    const disabledReason = cascadeState.disabledReasons[config.name];
    
    // 检查级联必填状态
    const cascadeRequired = cascadeState.required[config.name];
    const isRequired = cascadeRequired !== undefined ? cascadeRequired : config.required;
    
    const schemaWithRequired = {
      ...config.schema,
      required: isRequired,
    };
    
    // 最终禁用状态
    const isDisabled = config.disabled || disabled || cascadeDisabled;
    
    // 获取该字段的初始值（用于 DynamicObjectWidget 等需要区分存量数据的组件）
    const fieldInitialValue = mergedInitialValues[config.name];

    const fieldElement = (
      <Widget
        key={config.name}
        name={config.name}
        schema={schemaWithRequired}
        uiConfig={config.uiConfig}
        disabled={isDisabled}
        readOnly={readOnly}
        initialValue={fieldInitialValue}
        context={{
          values: form.getFieldsValue(),
          errors: {},
          touched: {},
          schema,
          providers,
          workspace,
          organization,
          manifest,
          workspaceResource,
          // 新增：手动触发值更新的回调
          triggerValuesUpdate,
        }}
      />
    );

    // 如果有禁用原因，显示提示
    if (cascadeDisabled && disabledReason) {
      return (
        <div key={config.name} className={styles.disabledFieldWrapper}>
          {fieldElement}
          <Tooltip title={disabledReason}>
            <InfoCircleOutlined className={styles.disabledReasonIcon} />
          </Tooltip>
        </div>
      );
    }

    return fieldElement;
  };

  // 将字段按 colSpan 分行：累加 colSpan 到 24 则换行
  const splitFieldsIntoRows = (fields: FieldRenderConfig[]): FieldRenderConfig[][] => {
    const rows: FieldRenderConfig[][] = [];
    let currentRow: FieldRenderConfig[] = [];
    let currentSpan = 0;

    for (const field of fields) {
      const span = field.uiConfig.colSpan || 24;
      if (currentSpan + span > 24 && currentRow.length > 0) {
        rows.push(currentRow);
        currentRow = [];
        currentSpan = 0;
      }
      currentRow.push(field);
      currentSpan += span;
      if (currentSpan >= 24) {
        rows.push(currentRow);
        currentRow = [];
        currentSpan = 0;
      }
    }
    if (currentRow.length > 0) {
      rows.push(currentRow);
    }
    return rows;
  };

  // 渲染分组内容
  const renderGroupContent = (group: GroupConfig & { fields: FieldRenderConfig[] }) => {
    const rows = splitFieldsIntoRows(group.fields);
    return (
      <div className={styles.fieldGroup}>
        {rows.map((rowFields, rowIdx) => (
          <Row gutter={[16, 0]} key={rowIdx}>
            {rowFields.map((field) => (
              <Col span={field.uiConfig.colSpan || 24} key={field.name}>
                {renderField(field)}
              </Col>
            ))}
          </Row>
        ))}
      </div>
    );
  };

  // 判断是否为受控模式
  const isControlled = activeGroupId !== undefined || onGroupChange !== undefined;

  // 处理 tab 切换
  const handleTabChange = useCallback((key: string) => {
    if (isControlled) {
      // 受控模式：调用外部回调
      onGroupChange?.(key);
    } else {
      // 非受控模式：更新内部状态
      setInternalActiveGroupId(key);
    }
  }, [isControlled, onGroupChange]);

  // 计算当前活跃的 tab
  const currentActiveKey = useMemo(() => {
    if (isControlled) {
      // 受控模式：使用外部传入的 activeGroupId
      if (activeGroupId && groupedFields.some(g => g.id === activeGroupId)) {
        return activeGroupId;
      }
    } else {
      // 非受控模式：使用内部状态
      if (internalActiveGroupId && groupedFields.some(g => g.id === internalActiveGroupId)) {
        return internalActiveGroupId;
      }
    }
    // 默认使用第一个分组
    return groupedFields[0]?.id;
  }, [isControlled, activeGroupId, internalActiveGroupId, groupedFields]);

  // 渲染标签页布局
  const renderTabsLayout = () => (
    <Tabs 
      activeKey={currentActiveKey}
      onChange={handleTabChange}
      className={styles.formTabs}
    >
      {groupedFields.map(group => (
        <TabPane
          key={group.id}
          tab={
            <span className={styles.tabHeader}>
              {group.title}
              <span className={styles.fieldCount}>{group.fields.length}</span>
            </span>
          }
        >
          {renderGroupContent(group)}
        </TabPane>
      ))}
    </Tabs>
  );

  // 渲染折叠面板布局
  const renderAccordionLayout = () => (
    <Collapse
      defaultActiveKey={groups.filter(g => g.defaultExpanded).map(g => g.id)}
      expandIconPosition="start"
      className={styles.formCollapse}
    >
      {groupedFields.map(group => (
        <Panel
          key={group.id}
          header={
            <span className={styles.groupHeader}>
              {group.title}
              <span className={styles.fieldCount}>{group.fields.length}</span>
            </span>
          }
        >
          {renderGroupContent(group)}
        </Panel>
      ))}
    </Collapse>
  );

  // 渲染分区布局（始终展开）
  const renderSectionsLayout = () => (
    <div className={styles.formSections}>
      {groupedFields.map(group => (
        <div key={group.id} className={styles.formSection}>
          <div className={styles.sectionHeader}>
            <span className={styles.sectionTitle}>{group.title}</span>
            <span className={styles.fieldCount}>{group.fields.length}</span>
          </div>
          {renderGroupContent(group)}
        </div>
      ))}
    </div>
  );

  // 渲染混合布局（每个分组使用自己的 layout）
  const renderMixedLayout = () => (
    <div className={styles.formMixed}>
      {groupedFields.map(group => {
        const layout = group.layout || 'sections';
        
        if (layout === 'tabs') {
          // 单个分组不适合用 tabs，降级为 sections
          return (
            <div key={group.id} className={styles.formSection}>
              <div className={styles.sectionHeader}>
                <span className={styles.sectionTitle}>{group.title}</span>
                <span className={styles.fieldCount}>{group.fields.length}</span>
              </div>
              {renderGroupContent(group)}
            </div>
          );
        }
        
        if (layout === 'accordion') {
          return (
            <Collapse
              key={group.id}
              defaultActiveKey={group.defaultExpanded ? [group.id] : []}
              expandIconPosition="start"
              className={styles.formCollapse}
            >
              <Panel
                key={group.id}
                header={
                  <span className={styles.groupHeader}>
                    {group.title}
                    <span className={styles.fieldCount}>{group.fields.length}</span>
                  </span>
                }
              >
                {renderGroupContent(group)}
              </Panel>
            </Collapse>
          );
        }
        
        // 默认 sections
        return (
          <div key={group.id} className={styles.formSection}>
            <div className={styles.sectionHeader}>
              <span className={styles.sectionTitle}>{group.title}</span>
              <span className={styles.fieldCount}>{group.fields.length}</span>
            </div>
            {renderGroupContent(group)}
          </div>
        );
      })}
    </div>
  );

  if (fieldConfigs.length === 0) {
    return <Empty description="暂无配置字段" />;
  }

  return (
    <Form
      form={form}
      layout="vertical"
      initialValues={mergedInitialValues}
      onValuesChange={handleValuesChange}
      onFinish={handleFinish}
      className={`${styles.formRenderer} ${readOnly ? styles.formRendererReadOnly : ''}`}
    >
      {/* AI 助手 */}
      {aiAssistant?.enabled && (
        <div className={styles.aiAssistantWrapper}>
          <AIConfigGenerator
            moduleId={aiAssistant.moduleId}
            workspaceId={aiAssistant.workspaceId}
            organizationId={aiAssistant.organizationId}
            manifestId={aiAssistant.manifestId}
            onGenerate={handleAIGenerate}
            disabled={disabled || readOnly}
          />
        </div>
      )}
      
      {globalLayout === 'tabs' && renderTabsLayout()}
      {globalLayout === 'accordion' && renderAccordionLayout()}
      {globalLayout === 'sections' && renderSectionsLayout()}
      {globalLayout === 'mixed' && renderMixedLayout()}
    </Form>
  );
};

export default FormRenderer;
