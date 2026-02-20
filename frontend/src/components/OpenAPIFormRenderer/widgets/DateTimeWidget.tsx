/**
 * DateTimeWidget - 日期时间选择器组件
 * 
 * 功能：
 * - 日期选择
 * - 时间选择
 * - 日期时间组合选择
 * - 时区支持
 */

import React, { useMemo } from 'react';
import { Form, DatePicker, Space, Typography } from 'antd';
import { CalendarOutlined } from '@ant-design/icons';
import type { WidgetProps } from '../types';
import dayjs from 'dayjs';

const { Text } = Typography;

// 日期格式类型
type DateFormat = 'date' | 'time' | 'datetime';

const DateTimeWidget: React.FC<WidgetProps> = ({
  name,
  schema,
  uiConfig,
  disabled,
  readOnly,
}) => {
  const form = Form.useFormInstance();
  const value = Form.useWatch(name, form);

  // 确定日期格式类型
  const formatType = useMemo((): DateFormat => {
    const uiConfigExt = uiConfig as { format?: string } | undefined;
    const format = schema.format || uiConfigExt?.format;
    if (format === 'date') return 'date';
    if (format === 'time') return 'time';
    return 'datetime';
  }, [schema.format, uiConfig]);

  // 获取显示格式
  const displayFormat = useMemo(() => {
    const uiConfigExt = uiConfig as { displayFormat?: string } | undefined;
    if (uiConfigExt?.displayFormat) return uiConfigExt.displayFormat;
    
    switch (formatType) {
      case 'date':
        return 'YYYY-MM-DD';
      case 'time':
        return 'HH:mm:ss';
      case 'datetime':
      default:
        return 'YYYY-MM-DD HH:mm:ss';
    }
  }, [formatType, uiConfig]);

  // 构建验证规则
  const rules = useMemo(() => {
    const ruleList: Array<{ required?: boolean; message?: string }> = [];
    
    if (schema.required) {
      ruleList.push({ required: true, message: `${schema.title || name} 是必填项` });
    }
    
    return ruleList;
  }, [schema, name]);

  // 渲染日期选择器
  const renderPicker = () => {
    const commonProps = {
      disabled: disabled || readOnly,
      format: displayFormat,
      placeholder: uiConfig?.placeholder || `请选择${schema.title || name}`,
      style: { width: '100%' },
      allowClear: true,
    };

    switch (formatType) {
      case 'date':
        return <DatePicker {...commonProps} />;
      case 'time':
        return <DatePicker.TimePicker {...commonProps} />;
      case 'datetime':
      default:
        return <DatePicker showTime {...commonProps} />;
    }
  };

  // 值转换：字符串 <-> dayjs
  const normalize = (val: unknown) => {
    if (!val) return undefined;
    if (dayjs.isDayjs(val)) return val;
    return dayjs(val as string);
  };

  const getValueFromEvent = (val: dayjs.Dayjs | null) => {
    if (!val) return undefined;
    // 根据格式类型返回不同格式的字符串
    switch (formatType) {
      case 'date':
        return val.format('YYYY-MM-DD');
      case 'time':
        return val.format('HH:mm:ss');
      case 'datetime':
      default:
        return val.toISOString();
    }
  };

  return (
    <Form.Item
      name={name}
      label={uiConfig?.label || schema.title || name}
      rules={rules}
      tooltip={schema.description}
      extra={uiConfig?.help}
      normalize={normalize}
      getValueFromEvent={getValueFromEvent}
    >
      {renderPicker()}
    </Form.Item>
  );
};

export default DateTimeWidget;
