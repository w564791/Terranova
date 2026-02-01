// Widget 组件导出
export { default as TextWidget } from './TextWidget';
export { default as TextareaWidget } from './TextareaWidget';
export { default as NumberWidget } from './NumberWidget';
export { default as SelectWidget } from './SelectWidget';
export { default as SwitchWidget } from './SwitchWidget';
export { default as TagsWidget } from './TagsWidget';
export { default as KeyValueWidget } from './KeyValueWidget';
export { default as ObjectWidget } from './ObjectWidget';
export { default as ObjectListWidget } from './ObjectListWidget';
export { default as DynamicObjectWidget } from './DynamicObjectWidget';
export { default as JsonEditorWidget } from './JsonEditorWidget';
export { default as PasswordWidget } from './PasswordWidget';
export { default as DateTimeWidget } from './DateTimeWidget';
export { default as CodeEditorWidget } from './CodeEditorWidget';
export { default as CMDBSelectWidget } from './CMDBSelectWidget';

import type { WidgetType } from '../types';
import TextWidget from './TextWidget';
import TextareaWidget from './TextareaWidget';
import NumberWidget from './NumberWidget';
import SelectWidget from './SelectWidget';
import SwitchWidget from './SwitchWidget';
import TagsWidget from './TagsWidget';
import KeyValueWidget from './KeyValueWidget';
import ObjectWidget from './ObjectWidget';
import ObjectListWidget from './ObjectListWidget';
import DynamicObjectWidget from './DynamicObjectWidget';
import JsonEditorWidget from './JsonEditorWidget';
import PasswordWidget from './PasswordWidget';
import DateTimeWidget from './DateTimeWidget';
import CodeEditorWidget from './CodeEditorWidget';
import CMDBSelectWidget from './CMDBSelectWidget';

// Widget 注册表
export const widgetRegistry: Record<WidgetType, React.ComponentType<any>> = {
  'text': TextWidget,
  'textarea': TextareaWidget, // 多行文本，支持数组类型（每行一个元素）
  'number': NumberWidget,
  'select': SelectWidget,
  'multi-select': SelectWidget,
  'switch': SwitchWidget,
  'tags': TagsWidget,
  'key-value': KeyValueWidget,
  'object': ObjectWidget, // 对象编辑器，支持嵌套属性递归渲染
  'object-list': ObjectListWidget, // 对象列表编辑器，支持添加/删除/复制
  'dynamic-object': DynamicObjectWidget, // 动态键对象编辑器 (CustomObject 类型)
  'json-editor': JsonEditorWidget, // JSON 编辑器，支持格式化和验证
  'password': PasswordWidget, // 密码输入，支持强度指示器
  'datetime': DateTimeWidget, // 日期时间选择器
  'code-editor': CodeEditorWidget, // 代码编辑器，支持多语言
  'cmdb-select': CMDBSelectWidget, // CMDB 资源选择器，从 CMDB 搜索资源
};

// 获取 Widget 组件
export function getWidget(type: WidgetType): React.ComponentType<any> {
  return widgetRegistry[type] || TextWidget;
}
