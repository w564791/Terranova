/**
 * HCL → JSON 解析器
 * 解析我们生成的 HCL 格式，转回配置数据
 * 支持的子集：字符串、数字、布尔、对象、数组（含嵌套对象数组）
 */

interface ParserState {
  input: string;
  pos: number;
}

// Terraform module block 系统参数（不提示用户）
export const TF_SYSTEM_PARAMS = new Set([
  'source',      // 模块来源
  'version',     // 模块版本
  'for_each',    // 循环创建
  'count',       // 条件/计数创建
  'depends_on',  // 显式依赖
  'providers',   // 提供者映射
  'lifecycle',   // 生命周期控制
]);

export interface HCLParseResult {
  moduleName: string;
  systemParams: Record<string, any>;  // source, version, for_each 等
  userConfig: Record<string, any>;    // 用户配置参数
}

function createParser(input: string): ParserState {
  return { input, pos: 0 };
}

function peek(state: ParserState): string {
  skipWhitespaceAndComments(state);
  return state.input[state.pos] || '';
}

function skipWhitespaceAndComments(state: ParserState): void {
  while (state.pos < state.input.length) {
    const ch = state.input[state.pos];
    if (ch === ' ' || ch === '\t' || ch === '\n' || ch === '\r') {
      state.pos++;
    } else if (ch === '#') {
      while (state.pos < state.input.length && state.input[state.pos] !== '\n') {
        state.pos++;
      }
    } else if (ch === '/' && state.input[state.pos + 1] === '/') {
      while (state.pos < state.input.length && state.input[state.pos] !== '\n') {
        state.pos++;
      }
    } else {
      break;
    }
  }
}

function expect(state: ParserState, expected: string): void {
  skipWhitespaceAndComments(state);
  if (state.input.slice(state.pos, state.pos + expected.length) !== expected) {
    throw new Error(`Expected '${expected}' at position ${state.pos}, got '${state.input.slice(state.pos, state.pos + 20)}'`);
  }
  state.pos += expected.length;
}

function parseIdentifier(state: ParserState): string {
  skipWhitespaceAndComments(state);
  let start = state.pos;
  while (state.pos < state.input.length && /[a-zA-Z0-9_-]/.test(state.input[state.pos])) {
    state.pos++;
  }
  if (state.pos === start) {
    throw new Error(`Expected identifier at position ${state.pos}`);
  }
  return state.input.slice(start, state.pos);
}

function parseString(state: ParserState): string {
  skipWhitespaceAndComments(state);
  if (state.input[state.pos] !== '"') {
    throw new Error(`Expected string at position ${state.pos}`);
  }
  state.pos++; // skip opening quote
  let result = '';
  while (state.pos < state.input.length) {
    const ch = state.input[state.pos];
    if (ch === '\\' && state.pos + 1 < state.input.length) {
      const next = state.input[state.pos + 1];
      if (next === '"') { result += '"'; state.pos += 2; }
      else if (next === '\\') { result += '\\'; state.pos += 2; }
      else if (next === 'n') { result += '\n'; state.pos += 2; }
      else if (next === 't') { result += '\t'; state.pos += 2; }
      else { result += ch; state.pos++; }
    } else if (ch === '"') {
      state.pos++; // skip closing quote
      return result;
    } else {
      result += ch;
      state.pos++;
    }
  }
  throw new Error('Unterminated string');
}

function parseNumber(state: ParserState): number {
  skipWhitespaceAndComments(state);
  let start = state.pos;
  if (state.input[state.pos] === '-') state.pos++;
  while (state.pos < state.input.length && /[0-9.]/.test(state.input[state.pos])) {
    state.pos++;
  }
  const num = parseFloat(state.input.slice(start, state.pos));
  if (isNaN(num)) {
    throw new Error(`Invalid number at position ${start}`);
  }
  return num;
}

function parseValue(state: ParserState): any {
  skipWhitespaceAndComments(state);
  const ch = state.input[state.pos];

  // String
  if (ch === '"') {
    return parseString(state);
  }

  // Object or block
  if (ch === '{') {
    return parseObject(state);
  }

  // Array
  if (ch === '[') {
    return parseArray(state);
  }

  // Boolean
  if (state.input.slice(state.pos, state.pos + 4) === 'true' &&
      !/[a-zA-Z0-9_]/.test(state.input[state.pos + 4] || '')) {
    state.pos += 4;
    return true;
  }
  if (state.input.slice(state.pos, state.pos + 5) === 'false' &&
      !/[a-zA-Z0-9_]/.test(state.input[state.pos + 5] || '')) {
    state.pos += 5;
    return false;
  }

  // Number (including negative)
  if (ch === '-' || (ch >= '0' && ch <= '9')) {
    return parseNumber(state);
  }

  throw new Error(`Unexpected character '${ch}' at position ${state.pos}`);
}

function parseObject(state: ParserState): Record<string, any> {
  expect(state, '{');
  const result: Record<string, any> = {};

  while (peek(state) !== '}') {
    if (state.pos >= state.input.length) {
      throw new Error('Unterminated object');
    }
    const key = parseIdentifier(state);
    expect(state, '=');
    result[key] = parseValue(state);
  }

  expect(state, '}');
  return result;
}

function parseArray(state: ParserState): any[] {
  expect(state, '[');
  const result: any[] = [];

  while (peek(state) !== ']') {
    if (state.pos >= state.input.length) {
      throw new Error('Unterminated array');
    }
    result.push(parseValue(state));
  }

  expect(state, ']');
  return result;
}

/**
 * 解析 module 块，分离系统参数和用户配置
 * 格式：module "name" { source = "..." version = "..." ... }
 */
export function parseHCLModule(hcl: string): HCLParseResult {
  const state = createParser(hcl.trim());

  // 跳过 module 关键字
  const keyword = parseIdentifier(state);
  if (keyword !== 'module') {
    throw new Error(`Expected 'module' block, got '${keyword}'`);
  }

  const moduleName = parseString(state);
  expect(state, '{');

  const systemParams: Record<string, any> = {};
  const userConfig: Record<string, any> = {};

  while (peek(state) !== '}') {
    if (state.pos >= state.input.length) {
      throw new Error('Unterminated module block');
    }
    const key = parseIdentifier(state);
    expect(state, '=');
    const value = parseValue(state);

    // 分离系统参数和用户配置
    if (TF_SYSTEM_PARAMS.has(key)) {
      systemParams[key] = value;
    } else {
      userConfig[key] = value;
    }
  }

  expect(state, '}');

  return { moduleName, systemParams, userConfig };
}

/**
 * 从 HCL 文本解析配置数据（只返回 userConfig 部分）
 */
export function parseHCLConfig(hcl: string): Record<string, any> {
  const { userConfig } = parseHCLModule(hcl);
  return userConfig;
}

/**
 * 检测 userConfig 中有哪些字段不在 schema 定义中
 */
export function detectExtraFields(
  userConfig: Record<string, any>,
  schema?: any
): string[] {
  if (!schema) return [];
  
  const schemaFields = new Set(
    Object.keys(schema?.components?.schemas?.ModuleInput?.properties || {})
  );
  
  if (schemaFields.size === 0) return [];
  
  return Object.keys(userConfig).filter(key => !schemaFields.has(key));
}
