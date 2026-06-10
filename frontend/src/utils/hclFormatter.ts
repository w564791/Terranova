/**
 * JSON → HCL 格式化工具
 * 将资源配置数据转换为 Terraform HCL 格式
 */

interface HCLFormatOptions {
  indent?: number;
  moduleName?: string;
  moduleSource?: string;
  moduleVersion?: string;
  schema?: any; // OpenAPI schema to detect defaults
  skipDefaults?: boolean; // skip fields with only default values
  systemParams?: Record<string, any>; // for_each, count, depends_on, providers, lifecycle
}

export function jsonToHCL(
  data: Record<string, unknown>,
  options: HCLFormatOptions = {}
): string {
  const {
    indent = 2,
    moduleName = 'resource',
    moduleSource,
    moduleVersion,
    schema,
    skipDefaults = false,
    systemParams = {},
  } = options;

  // Extract schema properties for format-aware rendering
  const schemaProperties: Record<string, any> = {};
  if (schema?.components?.schemas?.ModuleInput?.properties) {
    Object.assign(schemaProperties, schema.components.schemas.ModuleInput.properties);
  }

  // Extract defaults from schema if skipDefaults is enabled
  const schemaDefaults: Record<string, any> = {};
  if (skipDefaults && schema) {
    const properties = schema?.components?.schemas?.ModuleInput?.properties || {};
    Object.entries(properties).forEach(([key, prop]: [string, any]) => {
      if (prop.default !== undefined) {
        schemaDefaults[key] = prop.default;
      }
    });
  }

  // Filter data to exclude default values when skipDefaults is true
  let filteredData = data;
  if (skipDefaults && Object.keys(schemaDefaults).length > 0) {
    filteredData = {};
    Object.entries(data).forEach(([key, value]) => {
      const defaultValue = schemaDefaults[key];
      // Skip if value equals default (deep compare for objects/arrays)
      if (defaultValue !== undefined) {
        const isEqual = JSON.stringify(value) === JSON.stringify(defaultValue);
        if (!isEqual) {
          filteredData[key] = value;
        }
      } else {
        filteredData[key] = value;
      }
    });
  }

  const lines: string[] = [];
  const pad = (level: number) => ' '.repeat(indent * level);

  lines.push(`module "${moduleName}" {`);

  if (moduleSource) {
    lines.push(`${pad(1)}source  = "${escapeHCLString(moduleSource)}"`);
  }
  if (moduleVersion) {
    lines.push(`${pad(1)}version = "${escapeHCLString(moduleVersion)}"`);
  }

  // Render system params (for_each, count, depends_on, providers, lifecycle)
  const sysEntries = Object.entries(systemParams).filter(
    ([key]) => key !== 'source' && key !== 'version'
  );
  sysEntries.forEach(([key, value]) => {
    const formatted = formatValue(key, value, 1, pad);
    lines.push(...formatted);
  });

  if (moduleSource || moduleVersion || sysEntries.length > 0) {
    lines.push('');
  }

  const entries = Object.entries(filteredData).filter(
    ([key]) => key !== 'source' && key !== 'version'
  );

  entries.forEach(([key, value]) => {
    const schemaProp = schemaProperties[key];
    const formatted = formatValue(key, value, 1, pad, schemaProp);
    lines.push(...formatted);
  });

  lines.push('}');

  return lines.join('\n');
}

function formatValue(
  key: string,
  value: unknown,
  level: number,
  pad: (n: number) => string,
  schemaProp?: any
): string[] {
  const lines: string[] = [];
  const formattedKey = formatHCLKey(key);

  if (value === null || value === undefined) {
    lines.push(`${pad(level)}# ${formattedKey} = null`);
    return lines;
  }

  if (typeof value === 'boolean') {
    lines.push(`${pad(level)}${formattedKey} = ${value}`);
    return lines;
  }

  if (typeof value === 'number') {
    lines.push(`${pad(level)}${formattedKey} = ${value}`);
    return lines;
  }

  // 对象/数组值：如果 schema 标记为 format: json，强制用 jsonencode()
  const isJsonField = schemaProp?.format === 'json';
  if (isJsonField && typeof value === 'object') {
    lines.push(`${pad(level)}${formattedKey} = jsonencode(`);
    const inner = formatJsonEncodeValue(value, level + 1, pad);
    lines.push(...inner);
    lines.push(`${pad(level)})`);
    return lines;
  }

  if (typeof value === 'string') {
    // 检测是否为 JSON 字符串，如果是则渲染为 jsonencode({...})
    const trimmed = value.trim();
    if ((trimmed.startsWith('{') && trimmed.endsWith('}')) ||
        (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
      try {
        const parsed = JSON.parse(trimmed);
        lines.push(`${pad(level)}${formattedKey} = jsonencode(`);
        const inner = formatJsonEncodeValue(parsed, level + 1, pad);
        lines.push(...inner);
        lines.push(`${pad(level)})`);
        return lines;
      } catch {
        // 不是有效 JSON，按普通字符串处理
      }
    }
    lines.push(`${pad(level)}${formattedKey} = "${escapeHCLString(value)}"`);
    return lines;
  }

  if (Array.isArray(value)) {
    if (value.length === 0) {
      lines.push(`${pad(level)}${formattedKey} = []`);
      return lines;
    }

    const hasObjects = value.some(
      (item) => typeof item === 'object' && item !== null && !Array.isArray(item)
    );

    if (hasObjects) {
      lines.push(`${pad(level)}${formattedKey} = [`);
      value.forEach((item, idx) => {
        if (typeof item === 'object' && item !== null && !Array.isArray(item)) {
          lines.push(`${pad(level + 1)}{`);
          const entries = Object.entries(item);
          entries.forEach(([k, v]) => {
            const nested = formatValue(k, v, level + 2, pad);
            lines.push(...nested);
          });
          const comma = idx < value.length - 1 ? ',' : '';
          lines.push(`${pad(level + 1)}}${comma}`);
        } else {
          lines.push(`${pad(level + 1)}${formatPrimitive(item)},`);
        }
      });
      lines.push(`${pad(level)}]`);
    } else {
      // 简单数组：如果都是原始类型且数量少，单行输出
      const allSimple = value.every(
        (item) =>
          typeof item === 'string' ||
          typeof item === 'number' ||
          typeof item === 'boolean'
      );

      if (allSimple && value.length <= 4) {
        const items = value.map((v) => formatPrimitive(v)).join(', ');
        lines.push(`${pad(level)}${formattedKey} = [${items}]`);
      } else {
        lines.push(`${pad(level)}${formattedKey} = [`);
        value.forEach((item) => {
          lines.push(`${pad(level + 1)}${formatPrimitive(item)},`);
        });
        lines.push(`${pad(level)}]`);
      }
    }
    return lines;
  }

  if (typeof value === 'object') {
    const entries = Object.entries(value);
    if (entries.length === 0) {
      lines.push(`${pad(level)}${formattedKey} = {}`);
      return lines;
    }

    lines.push(`${pad(level)}${formattedKey} = {`);
    entries.forEach(([k, v]) => {
      const nested = formatValue(k, v, level + 1, pad);
      lines.push(...nested);
    });
    lines.push(`${pad(level)}}`);
    return lines;
  }

  lines.push(`${pad(level)}${formattedKey} = "${String(value)}"`);
  return lines;
}

/**
 * jsonencode 内部的值格式化 — HCL 风格（= 分隔，末尾无逗号）
 */
function formatJsonEncodeValue(
  value: unknown,
  level: number,
  pad: (n: number) => string
): string[] {
  const lines: string[] = [];

  if (value === null || value === undefined) {
    lines.push(`${pad(level)}null`);
    return lines;
  }

  if (typeof value === 'boolean') {
    lines.push(`${pad(level)}${value}`);
    return lines;
  }

  if (typeof value === 'number') {
    lines.push(`${pad(level)}${value}`);
    return lines;
  }

  if (typeof value === 'string') {
    lines.push(`${pad(level)}"${escapeHCLString(value)}"`);
    return lines;
  }

  if (Array.isArray(value)) {
    if (value.length === 0) {
      lines.push(`${pad(level)}[]`);
      return lines;
    }
    lines.push(`${pad(level)}[`);
    value.forEach((item, idx) => {
      if (typeof item === 'object' && item !== null && !Array.isArray(item)) {
        lines.push(`${pad(level + 1)}{`);
        const entries = Object.entries(item);
        entries.forEach(([k, v]) => {
          const nested = formatJsonEncodeKV(k, v, level + 2, pad);
          lines.push(...nested);
        });
        const comma = idx < value.length - 1 ? ',' : '';
        lines.push(`${pad(level + 1)}}${comma}`);
      } else if (Array.isArray(item)) {
        const inner = formatJsonEncodeValue(item, level + 1, pad);
        lines.push(...inner);
        if (idx < value.length - 1) {
          lines[lines.length - 1] += ',';
        }
      } else {
        const primitive = formatJsonEncodePrimitive(item);
        const comma = idx < value.length - 1 ? ',' : '';
        lines.push(`${pad(level + 1)}${primitive}${comma}`);
      }
    });
    lines.push(`${pad(level)}]`);
    return lines;
  }

  if (typeof value === 'object') {
    const entries = Object.entries(value);
    if (entries.length === 0) {
      lines.push(`${pad(level)}{}`);
      return lines;
    }
    lines.push(`${pad(level)}{`);
    entries.forEach(([k, v]) => {
      const nested = formatJsonEncodeKV(k, v, level + 1, pad);
      lines.push(...nested);
    });
    lines.push(`${pad(level)}}`);
    return lines;
  }

  lines.push(`${pad(level)}"${String(value)}"`);
  return lines;
}

/**
 * 判断 HCL key 是否需要引号
 * 合法标识符：只包含 [a-zA-Z0-9_-] 且以字母或下划线开头
 */
function needsQuoteForKey(key: string): boolean {
  if (!key || key.length === 0) return true;
  // 必须以字母或下划线开头
  if (!/^[a-zA-Z_]/.test(key)) return true;
  // 只能包含字母、数字、下划线和短横线
  if (!/^[a-zA-Z0-9_-]+$/.test(key)) return true;
  return false;
}

/**
 * 格式化 HCL key，必要时加引号
 */
function formatHCLKey(key: string): string {
  if (needsQuoteForKey(key)) {
    return `"${key.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
  }
  return key;
}

/**
 * jsonencode 内部的 key = value 格式化
 */
function formatJsonEncodeKV(
  key: string,
  value: unknown,
  level: number,
  pad: (n: number) => string
): string[] {
  const lines: string[] = [];
  const formattedKey = formatHCLKey(key);

  if (value === null || value === undefined) {
    lines.push(`${pad(level)}${formattedKey} = null`);
    return lines;
  }

  if (typeof value === 'boolean' || typeof value === 'number') {
    lines.push(`${pad(level)}${formattedKey} = ${value}`);
    return lines;
  }

  if (typeof value === 'string') {
    lines.push(`${pad(level)}${formattedKey} = "${escapeHCLString(value)}"`);
    return lines;
  }

  if (Array.isArray(value)) {
    lines.push(`${pad(level)}${formattedKey} = [`);
    value.forEach((item, idx) => {
      if (typeof item === 'object' && item !== null && !Array.isArray(item)) {
        lines.push(`${pad(level + 1)}{`);
        Object.entries(item).forEach(([k, v]) => {
          const nested = formatJsonEncodeKV(k, v, level + 2, pad);
          lines.push(...nested);
        });
        const comma = idx < value.length - 1 ? ',' : '';
        lines.push(`${pad(level + 1)}}${comma}`);
      } else {
        const primitive = formatJsonEncodePrimitive(item);
        const comma = idx < value.length - 1 ? ',' : '';
        lines.push(`${pad(level + 1)}${primitive}${comma}`);
      }
    });
    lines.push(`${pad(level)}]`);
    return lines;
  }

  if (typeof value === 'object') {
    lines.push(`${pad(level)}${formattedKey} = {`);
    Object.entries(value).forEach(([k, v]) => {
      const nested = formatJsonEncodeKV(k, v, level + 1, pad);
      lines.push(...nested);
    });
    lines.push(`${pad(level)}}`);
    return lines;
  }

  lines.push(`${pad(level)}${formattedKey} = "${String(value)}"`);
  return lines;
}

function formatJsonEncodePrimitive(value: unknown): string {
  if (typeof value === 'string') return `"${escapeHCLString(value)}"`;
  if (typeof value === 'boolean') return String(value);
  if (typeof value === 'number') return String(value);
  if (value === null || value === undefined) return 'null';
  return `"${String(value)}"`;
}

function formatPrimitive(value: unknown): string {
  if (typeof value === 'string') return `"${escapeHCLString(value)}"`;
  if (typeof value === 'boolean') return String(value);
  if (typeof value === 'number') return String(value);
  if (value === null || value === undefined) return 'null';
  return `"${String(value)}"`;
}

function escapeHCLString(str: string): string {
  return str.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '\\n');
}

/**
 * HCL 语法高亮 — 逐行处理，token 级匹配，不破坏多行结构
 */
export function highlightHCL(hcl: string): string {
  return hcl.split('\n').map(highlightLine).join('\n');
}

function highlightLine(line: string): string {
  const trimmed = line.trim();
  if (!trimmed) return '';

  // 注释
  if (trimmed.startsWith('#')) {
    return `<span class="hcl-comment">${escapeHTML(line)}</span>`;
  }

  // 提取缩进
  const indent = line.length - line.trimStart().length;
  const indentStr = line.substring(0, indent);

  // 闭合括号: }  ]  ),  },  ],  }]  ]}
  if (/^[\}\)\]][\}\)\],]*,?$/.test(trimmed)) {
    const body = escapeHTML(trimmed);
    return `${escapeHTML(indentStr)}${body.replace(/[\{\}\[\]\(\)]/g, '<span class="hcl-bracket">$&</span>')}`;
  }

  // 开括号: {  [
  if (trimmed === '{' || trimmed === '[') {
    return `${escapeHTML(indentStr)}<span class="hcl-bracket">${trimmed}</span>`;
  }

  // 块声明: module "name" { / resource "type" "name" {
  const blockRe = /^(\s*)(module|resource|data|variable|output|locals|provider|terraform)\b(.*)$/;
  const blockM = line.match(blockRe);
  if (blockM) {
    const [, ws, kw, rest] = blockM;
    const highlighted = escapeHTML(rest)
      .replace(/"([^"]*)"/g, '<span class="hcl-string">"$1"</span>')
      .replace(/\{/, '<span class="hcl-bracket">{</span>');
    return `${escapeHTML(ws)}<span class="hcl-keyword">${kw}</span>${highlighted}`;
  }

  // 属性赋值: key = value
  const attrRe = /^(\s*)([a-zA-Z_][a-zA-Z0-9_-]*)\s*=\s*(.*)$/;
  const attrM = line.match(attrRe);
  if (attrM) {
    const [, ws, attr, val] = attrM;
    return `${escapeHTML(ws)}<span class="hcl-attr">${escapeHTML(attr)}</span> <span class="hcl-eq">=</span> ${highlightValue(val)}`;
  }

  // 兜底：原样转义
  return escapeHTML(line);
}

function highlightValue(raw: string): string {
  const v = raw.trimEnd();
  if (!v) return '';

  // jsonencode(...)
  if (v.startsWith('jsonencode(')) {
    return `<span class="hcl-keyword">jsonencode</span><span class="hcl-bracket">(</span>`;
  }
  // closing jsonencode paren
  if (v === ')') {
    return `<span class="hcl-bracket">)</span>`;
  }
  // boolean
  if (v === 'true' || v === 'false') return `<span class="hcl-bool">${v}</span>`;
  // number
  if (/^-?\d+(\.\d+)?$/.test(v)) return `<span class="hcl-number">${v}</span>`;
  // string
  if (v.startsWith('"')) return `<span class="hcl-string">${escapeHTML(v)}</span>`;
  // 空容器
  if (v === '{}' || v === '[]') return `<span class="hcl-bracket">${v}</span>`;
  // 开括号（多行 object/array 的首行）
  if (v === '{' || v === '[') return `<span class="hcl-bracket">${v}</span>`;

  // 兜底
  return escapeHTML(v);
}

function escapeHTML(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}
