import React, { useState, useMemo, useEffect } from 'react';
import {
  parseStateResources,
  searchResources,
  filterResourcesByMode,
  extractResourceDisplayName,
} from '../../utils/stateParser';
import type {
  StateContent,
  ParsedResource,
} from '../../utils/stateParser';

// 从 ParsedResource 中提取 instance 类型
type ResourceInstance = ParsedResource['instances'][0];
import styles from './StateResourceViewer.module.css';

interface StateResourceViewerProps {
  stateContent: StateContent;
  showSensitive?: boolean;
}

// Copy to clipboard helper
const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
};

// 按资源显示名称分组
interface ResourceGroup {
  displayName: string;
  resources: ParsedResource[];
}

function groupResourcesByDisplayName(resources: ParsedResource[]): ResourceGroup[] {
  const groupMap = new Map<string, ParsedResource[]>();
  
  resources.forEach(resource => {
    const displayName = extractResourceDisplayName(resource);
    if (!groupMap.has(displayName)) {
      groupMap.set(displayName, []);
    }
    groupMap.get(displayName)!.push(resource);
  });
  
  const groups: ResourceGroup[] = [];
  groupMap.forEach((groupResources, displayName) => {
    groups.push({
      displayName,
      resources: groupResources,
    });
  });
  
  // 按名称排序，Root Module 在前
  groups.sort((a, b) => {
    if (a.displayName === 'Root Module') return -1;
    if (b.displayName === 'Root Module') return 1;
    return a.displayName.localeCompare(b.displayName);
  });
  
  return groups;
}

const StateResourceViewer: React.FC<StateResourceViewerProps> = ({
  stateContent,
  showSensitive = false,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [modeFilter, setModeFilter] = useState<'all' | 'data' | 'managed'>('all');
  const [expandAllNodes, setExpandAllNodes] = useState<boolean | undefined>(undefined);

  // 解析资源
  const allResources = useMemo(() => {
    return parseStateResources(stateContent);
  }, [stateContent]);

  // 过滤和搜索
  const filteredResources = useMemo(() => {
    let result = allResources;
    
    // 按模式过滤
    result = filterResourcesByMode(result, modeFilter);
    
    // 搜索
    if (searchQuery) {
      result = searchResources(result, searchQuery, { searchInAttributes: true });
    }
    
    return result;
  }, [allResources, modeFilter, searchQuery]);

  // 按显示名称分组
  const resourceGroups = useMemo(() => {
    return groupResourcesByDisplayName(filteredResources);
  }, [filteredResources]);

  // 展开所有
  const handleExpandAll = () => {
    setExpandAllNodes(true);
  };

  // 折叠所有
  const handleCollapseAll = () => {
    setExpandAllNodes(false);
  };

  return (
    <div className={styles.container}>
      {/* 搜索和过滤 */}
      <div className={styles.searchSection}>
        <div className={styles.searchForm}>
          <input
            type="text"
            placeholder="Search resources..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className={styles.searchInput}
          />
          <select
            value={modeFilter}
            onChange={(e) => setModeFilter(e.target.value as 'all' | 'data' | 'managed')}
            className={styles.filterSelect}
          >
            <option value="all">All Types</option>
            <option value="managed">Managed Only</option>
            <option value="data">Data Sources Only</option>
          </select>
          <button onClick={handleExpandAll} className={styles.actionButton}>
            Expand All
          </button>
          <button onClick={handleCollapseAll} className={styles.actionButton}>
            Collapse All
          </button>
        </div>
      </div>

      {/* 资源列表 */}
      <div className={styles.resourceList}>
        {resourceGroups.length === 0 ? (
          <div className={styles.emptyState}>
            <p className={styles.emptyText}>
              {searchQuery ? 'No resources match your search.' : 'No resources found in state.'}
            </p>
          </div>
        ) : (
          resourceGroups.map((group) => (
            <ResourceGroupCard
              key={group.displayName}
              group={group}
              expandAll={expandAllNodes}
              showSensitive={showSensitive}
            />
          ))
        )}
      </div>
    </div>
  );
};

// 第一级：资源组卡片（按显示名称分组）
interface ResourceGroupCardProps {
  group: ResourceGroup;
  expandAll?: boolean;
  showSensitive: boolean;
}

const ResourceGroupCard: React.FC<ResourceGroupCardProps> = ({
  group,
  expandAll,
  showSensitive,
}) => {
  const [expanded, setExpanded] = useState(false);

  // 响应 expandAll 变化
  useEffect(() => {
    if (expandAll !== undefined) {
      setExpanded(expandAll);
    }
  }, [expandAll]);

  return (
    <div className={styles.resourceCard}>
      <div
        className={styles.resourceCardHeader}
        onClick={() => setExpanded(!expanded)}
      >
        <span className={styles.expandIcon}>{expanded ? '▼' : '▶'}</span>
        <span className={styles.resourceName}>{group.displayName}</span>
        <span className={styles.resourceCount}>
          {group.resources.length} resources
        </span>
      </div>
      
      {expanded && (
        <div className={styles.resourceCardContent}>
          {group.resources.map((resource) => (
            <AddressItem
              key={resource.address}
              resource={resource}
              expandAll={expandAll}
              showSensitive={showSensitive}
            />
          ))}
        </div>
      )}
    </div>
  );
};

// 第二级：地址项（完整地址）
interface AddressItemProps {
  resource: ParsedResource;
  expandAll?: boolean;
  showSensitive: boolean;
}

const AddressItem: React.FC<AddressItemProps> = ({
  resource,
  expandAll,
  showSensitive,
}) => {
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);

  // 响应 expandAll 变化
  useEffect(() => {
    if (expandAll !== undefined) {
      setExpanded(expandAll);
    }
  }, [expandAll]);

  const handleCopyAddress = async (e: React.MouseEvent) => {
    e.stopPropagation();
    const success = await copyToClipboard(resource.address);
    if (success) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <div className={styles.addressItem}>
      <div
        className={styles.addressHeader}
        onClick={() => setExpanded(!expanded)}
      >
        <span className={styles.expandIcon}>{expanded ? '▼' : '▶'}</span>
        <span className={`${styles.modeIcon} ${resource.mode === 'data' ? styles.dataIcon : styles.managedIcon}`}>
          {resource.mode === 'data' ? 'D' : 'R'}
        </span>
        <span className={styles.addressText}>{resource.address}</span>
        <span 
          className={styles.typeTag}
          onClick={handleCopyAddress}
          title="Click to copy address"
        >
          {resource.type}
          {copied && <span className={styles.copiedToast}>Copied!</span>}
        </span>
      </div>
      
      {expanded && (
        <div className={styles.addressContent}>
          {resource.instances.map((instance, idx) => (
            <InstanceDetails
              key={idx}
              instance={instance}
              resource={resource}
              showIndex={resource.instances.length > 1}
              showSensitive={showSensitive}
            />
          ))}
        </div>
      )}
    </div>
  );
};

// 第三级：实例详情（类似 terraform state show 的完整输出）
interface InstanceDetailsProps {
  instance: ResourceInstance;
  resource: ParsedResource;
  showIndex: boolean;
  showSensitive: boolean;
}

const InstanceDetails: React.FC<InstanceDetailsProps> = ({
  instance,
  resource,
  showIndex,
  showSensitive,
}) => {
  const [copied, setCopied] = useState(false);

  const isSensitive = (key: string): boolean => {
    return instance.sensitiveAttributes.some((attr: string) => {
      try {
        const parsed = JSON.parse(attr);
        if (Array.isArray(parsed)) {
          return parsed[0] === key;
        }
      } catch {
        return attr === key;
      }
      return false;
    });
  };

  // 计算最长的属性名长度，用于对齐
  const getMaxKeyLength = (attrs: Record<string, any>): number => {
    return Math.max(...Object.keys(attrs).map(k => k.length), 0);
  };

  // 格式化简单值
  const formatSimpleValue = (value: any, key: string): string => {
    if (isSensitive(key) && !showSensitive) {
      return '(sensitive value)';
    }
    
    if (value === null) return 'null';
    if (typeof value === 'boolean') return String(value);
    if (typeof value === 'number') return String(value);
    if (typeof value === 'string') {
      if (value === '') return '""';
      if (value.includes('\n')) {
        return `<<-EOT\n${value}\nEOT`;
      }
      return `"${value}"`;
    }
    return String(value);
  };

  // 格式化数组值
  const formatArrayValue = (arr: any[], key: string, indent: string): string => {
    if (arr.length === 0) return '[]';
    
    // 检查是否是简单值数组
    const isSimpleArray = arr.every(item => 
      typeof item !== 'object' || item === null
    );
    
    if (isSimpleArray) {
      const items = arr.map(item => `${indent}    ${formatSimpleValue(item, key)}`);
      return `[\n${items.join(',\n')}\n${indent}]`;
    }
    
    // 复杂对象数组 - 作为嵌套块处理
    return ''; // 将在 formatBlock 中处理
  };

  // 格式化对象值
  const formatObjectValue = (obj: Record<string, any>, indent: string): string => {
    const keys = Object.keys(obj);
    if (keys.length === 0) return '{}';
    
    const maxLen = getMaxKeyLength(obj);
    const entries = keys.map(k => {
      const v = obj[k];
      const padding = ' '.repeat(maxLen - k.length);
      
      if (v === null) {
        return `${indent}    ${k}${padding} = null`;
      }
      if (typeof v !== 'object') {
        return `${indent}    ${k}${padding} = ${formatSimpleValue(v, k)}`;
      }
      if (Array.isArray(v)) {
        if (v.length === 0) {
          return `${indent}    ${k}${padding} = []`;
        }
        const arrStr = formatArrayValue(v, k, indent + '    ');
        if (arrStr) {
          return `${indent}    ${k}${padding} = ${arrStr}`;
        }
        return null; // 复杂数组作为嵌套块
      }
      // 嵌套对象
      return `${indent}    ${k}${padding} = ${formatObjectValue(v, indent + '    ')}`;
    }).filter(Boolean);
    
    return `{\n${entries.join('\n')}\n${indent}}`;
  };

  // 格式化嵌套块（terraform state show 风格）
  const formatBlock = (name: string, value: any, indent: string): string[] => {
    const lines: string[] = [];
    
    if (Array.isArray(value)) {
      // 数组中的每个对象作为单独的块
      for (const item of value) {
        if (typeof item === 'object' && item !== null) {
          lines.push('');
          lines.push(`${indent}${name} {`);
          const blockLines = formatAttributes(item, indent + '    ');
          lines.push(...blockLines);
          lines.push(`${indent}}`);
        }
      }
    } else if (typeof value === 'object' && value !== null) {
      lines.push('');
      lines.push(`${indent}${name} {`);
      const blockLines = formatAttributes(value, indent + '    ');
      lines.push(...blockLines);
      lines.push(`${indent}}`);
    }
    
    return lines;
  };

  // 格式化属性列表
  const formatAttributes = (attrs: Record<string, any>, indent: string): string[] => {
    const lines: string[] = [];
    const sortedKeys = Object.keys(attrs).sort();
    const maxLen = getMaxKeyLength(attrs);
    
    // 分离简单属性和复杂属性（块）
    const simpleAttrs: string[] = [];
    const blockAttrs: { key: string; value: any }[] = [];
    
    for (const key of sortedKeys) {
      const value = attrs[key];
      
      // 跳过 null 和空数组（除非是特定字段）
      if (value === null && !['timeouts'].includes(key)) {
        continue;
      }
      
      if (Array.isArray(value)) {
        if (value.length === 0) {
          // 空数组显示为 []
          const padding = ' '.repeat(maxLen - key.length);
          simpleAttrs.push(`${indent}${key}${padding} = []`);
        } else if (value.every(item => typeof item !== 'object' || item === null)) {
          // 简单值数组
          const padding = ' '.repeat(maxLen - key.length);
          const items = value.map(item => `${indent}    ${formatSimpleValue(item, key)}`);
          simpleAttrs.push(`${indent}${key}${padding} = [\n${items.join(',\n')}\n${indent}]`);
        } else {
          // 复杂对象数组 - 作为块处理
          blockAttrs.push({ key, value });
        }
      } else if (typeof value === 'object' && value !== null) {
        // 检查是否应该作为块显示
        const isBlock = Object.keys(value).some(k => 
          typeof value[k] === 'object' && value[k] !== null
        );
        
        if (isBlock) {
          blockAttrs.push({ key, value });
        } else {
          // 简单对象
          const padding = ' '.repeat(maxLen - key.length);
          simpleAttrs.push(`${indent}${key}${padding} = ${formatObjectValue(value, indent)}`);
        }
      } else {
        // 简单值
        const padding = ' '.repeat(maxLen - key.length);
        simpleAttrs.push(`${indent}${key}${padding} = ${formatSimpleValue(value, key)}`);
      }
    }
    
    // 先添加简单属性
    lines.push(...simpleAttrs);
    
    // 再添加块属性
    for (const { key, value } of blockAttrs) {
      const blockLines = formatBlock(key, value, indent);
      lines.push(...blockLines);
    }
    
    return lines;
  };

  // 生成类似 terraform state show 的输出
  const generateStateShowOutput = (): string => {
    const lines: string[] = [];
    
    // 资源头部注释
    let addressComment = resource.address;
    if (instance.indexKey !== undefined) {
      const indexStr = typeof instance.indexKey === 'string' 
        ? `["${instance.indexKey}"]` 
        : `[${instance.indexKey}]`;
      addressComment += indexStr;
    }
    lines.push(`# ${addressComment}:`);
    
    // 资源声明
    const resourceDecl = resource.mode === 'data' 
      ? `data "${resource.type}" "${resource.name}"`
      : `resource "${resource.type}" "${resource.name}"`;
    lines.push(`${resourceDecl} {`);
    
    // 属性
    const attrLines = formatAttributes(instance.attributes, '    ');
    lines.push(...attrLines);
    
    lines.push('}');
    
    return lines.join('\n');
  };

  const stateShowOutput = generateStateShowOutput();

  const handleCopy = async () => {
    const success = await copyToClipboard(stateShowOutput);
    if (success) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <div className={styles.instanceDetails}>
      <div className={styles.instanceToolbar}>
        <button 
          className={styles.copyButton}
          onClick={handleCopy}
          title="Copy to clipboard"
        >
          {copied ? 'Copied!' : 'Copy'}
        </button>
      </div>
      <pre className={styles.stateShowOutput}>
        <code>{stateShowOutput}</code>
      </pre>
    </div>
  );
};

export default StateResourceViewer;
