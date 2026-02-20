/**
 * CascadeEngine - 级联规则引擎
 * 
 * 用于处理表单字段间的联动效果：
 * - 字段显示/隐藏
 * - 字段启用/禁用
 * - 字段值设置
 * - 字段选项更新
 * - 字段必填状态
 */

import type { CascadeRule, CascadeAction } from './types';

// 级联状态
export interface CascadeState {
  // 字段可见性状态
  visibility: Record<string, boolean>;
  // 字段禁用状态
  disabled: Record<string, boolean>;
  // 字段禁用原因
  disabledReasons: Record<string, string>;
  // 字段必填状态（由级联规则动态设置）
  required: Record<string, boolean>;
  // 待设置的字段值
  pendingValues: Record<string, unknown>;
}

// 操作符类型
type Operator = 
  | 'eq' | 'ne' 
  | 'gt' | 'lt' | 'gte' | 'lte'
  | 'in' | 'notIn'
  | 'empty' | 'notEmpty'
  | 'contains' | 'startsWith' | 'endsWith'
  | 'matches';

/**
 * 级联规则引擎
 */
export class CascadeEngine {
  private rules: CascadeRule[];
  private initialVisibility: Record<string, boolean>;
  
  constructor(rules: CascadeRule[] = [], initialVisibility: Record<string, boolean> = {}) {
    this.rules = rules;
    this.initialVisibility = initialVisibility;
  }

  /**
   * 更新规则
   */
  setRules(rules: CascadeRule[]): void {
    this.rules = rules;
  }

  /**
   * 设置初始可见性
   */
  setInitialVisibility(visibility: Record<string, boolean>): void {
    this.initialVisibility = visibility;
  }

  /**
   * 评估所有规则，返回级联状态
   */
  evaluate(formValues: Record<string, unknown>): CascadeState {
    const state: CascadeState = {
      visibility: { ...this.initialVisibility },
      disabled: {},
      disabledReasons: {},
      required: {},
      pendingValues: {},
    };

    // 遍历所有规则
    for (const rule of this.rules) {
      const triggered = this.evaluateTrigger(rule.trigger, formValues);
      
      if (triggered) {
        // 执行所有动作
        for (const action of rule.actions) {
          this.executeAction(action, state);
        }
      }
    }

    return state;
  }

  /**
   * 评估触发条件
   */
  private evaluateTrigger(
    trigger: CascadeRule['trigger'],
    formValues: Record<string, unknown>
  ): boolean {
    const { field, operator, value } = trigger;
    const fieldValue = this.getFieldValue(formValues, field);
    
    return this.evaluateOperator(operator as Operator, fieldValue, value);
  }

  /**
   * 获取字段值（支持嵌套路径）
   */
  private getFieldValue(formValues: Record<string, unknown>, fieldPath: string): unknown {
    const parts = fieldPath.split('.');
    let value: unknown = formValues;
    
    for (const part of parts) {
      if (value === null || value === undefined) {
        return undefined;
      }
      value = (value as Record<string, unknown>)[part];
    }
    
    return value;
  }

  /**
   * 评估操作符
   */
  private evaluateOperator(operator: Operator, fieldValue: unknown, compareValue: unknown): boolean {
    switch (operator) {
      case 'eq':
        return fieldValue === compareValue;
      
      case 'ne':
        return fieldValue !== compareValue;
      
      case 'gt':
        return typeof fieldValue === 'number' && typeof compareValue === 'number' 
          && fieldValue > compareValue;
      
      case 'lt':
        return typeof fieldValue === 'number' && typeof compareValue === 'number' 
          && fieldValue < compareValue;
      
      case 'gte':
        return typeof fieldValue === 'number' && typeof compareValue === 'number' 
          && fieldValue >= compareValue;
      
      case 'lte':
        return typeof fieldValue === 'number' && typeof compareValue === 'number' 
          && fieldValue <= compareValue;
      
      case 'in':
        return Array.isArray(compareValue) && compareValue.includes(fieldValue);
      
      case 'notIn':
        return Array.isArray(compareValue) && !compareValue.includes(fieldValue);
      
      case 'empty':
        return this.isEmpty(fieldValue);
      
      case 'notEmpty':
        return !this.isEmpty(fieldValue);
      
      case 'contains':
        return typeof fieldValue === 'string' && typeof compareValue === 'string'
          && fieldValue.includes(compareValue);
      
      case 'startsWith':
        return typeof fieldValue === 'string' && typeof compareValue === 'string'
          && fieldValue.startsWith(compareValue);
      
      case 'endsWith':
        return typeof fieldValue === 'string' && typeof compareValue === 'string'
          && fieldValue.endsWith(compareValue);
      
      case 'matches':
        if (typeof fieldValue !== 'string' || typeof compareValue !== 'string') {
          return false;
        }
        try {
          const regex = new RegExp(compareValue);
          return regex.test(fieldValue);
        } catch {
          return false;
        }
      
      default:
        console.warn(`Unknown operator: ${operator}`);
        return false;
    }
  }

  /**
   * 检查值是否为空
   */
  private isEmpty(value: unknown): boolean {
    if (value === null || value === undefined) {
      return true;
    }
    if (typeof value === 'string') {
      return value.trim() === '';
    }
    if (Array.isArray(value)) {
      return value.length === 0;
    }
    if (typeof value === 'object') {
      return Object.keys(value).length === 0;
    }
    return false;
  }

  /**
   * 执行动作
   */
  private executeAction(action: CascadeAction, state: CascadeState): void {
    switch (action.type) {
      case 'show':
        if (action.fields) {
          for (const field of action.fields) {
            state.visibility[field] = true;
          }
        }
        break;
      
      case 'hide':
        if (action.fields) {
          for (const field of action.fields) {
            state.visibility[field] = false;
          }
        }
        break;
      
      case 'enable':
        if (action.fields) {
          for (const field of action.fields) {
            state.disabled[field] = false;
            delete state.disabledReasons[field];
          }
        }
        break;
      
      case 'disable':
        if (action.fields) {
          for (const field of action.fields) {
            state.disabled[field] = true;
            if (action.message) {
              state.disabledReasons[field] = action.message;
            }
          }
        }
        break;
      
      case 'setValue':
        if (action.field !== undefined && action.value !== undefined) {
          state.pendingValues[action.field] = action.value;
        }
        break;
      
      case 'setRequired':
        if (action.fields) {
          const required = (action as { required?: boolean }).required ?? true;
          for (const field of action.fields) {
            state.required[field] = required;
          }
        }
        break;
      
      case 'clearValue':
        if (action.fields) {
          for (const field of action.fields) {
            state.pendingValues[field] = undefined;
          }
        }
        break;
      
      case 'setOptions':
        // setOptions 需要在 Widget 层面处理，这里只记录
        // 可以通过 state 扩展来支持
        break;
      
      case 'reloadSource':
        // reloadSource 需要在 FormRenderer 层面处理
        // 可以通过回调或事件来实现
        break;
      
      default:
        console.warn(`Unknown action type: ${action.type}`);
    }
  }

  /**
   * 获取字段的可见性
   */
  getFieldVisibility(state: CascadeState, fieldName: string): boolean {
    // 如果在 visibility 中有明确设置，使用该值
    if (fieldName in state.visibility) {
      return state.visibility[fieldName];
    }
    // 否则默认可见
    return true;
  }

  /**
   * 获取字段的禁用状态
   */
  getFieldDisabled(state: CascadeState, fieldName: string): boolean {
    return state.disabled[fieldName] ?? false;
  }

  /**
   * 获取字段的禁用原因
   */
  getFieldDisabledReason(state: CascadeState, fieldName: string): string | undefined {
    return state.disabledReasons[fieldName];
  }

  /**
   * 获取字段的必填状态（由级联规则设置）
   */
  getFieldRequired(state: CascadeState, fieldName: string): boolean | undefined {
    return state.required[fieldName];
  }

  /**
   * 获取待设置的字段值
   */
  getPendingValues(state: CascadeState): Record<string, unknown> {
    return state.pendingValues;
  }
}

/**
 * 创建级联引擎实例
 */
export function createCascadeEngine(
  rules: CascadeRule[] = [],
  initialVisibility: Record<string, boolean> = {}
): CascadeEngine {
  return new CascadeEngine(rules, initialVisibility);
}

/**
 * React Hook: 使用级联引擎
 */
export function useCascadeEngine(
  rules: CascadeRule[],
  formValues: Record<string, unknown>,
  initialVisibility: Record<string, boolean> = {}
): CascadeState {
  const engine = new CascadeEngine(rules, initialVisibility);
  return engine.evaluate(formValues);
}

export default CascadeEngine;
