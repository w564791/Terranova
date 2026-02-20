import React, { useState, useEffect, useMemo } from 'react';
import { Popover, Input, List, Typography, Tag, Empty, Spin } from 'antd';
import { AppstoreOutlined, ApiOutlined, CloudOutlined } from '@ant-design/icons';
import { schemaV2Service } from '../../services/schemaV2';
import type { RemoteDataNode, RemoteDataOutput } from '../OpenAPIFormRenderer/types';
import styles from './ModuleReferencePopover.module.css';

const { Text } = Typography;

// Module 节点信息
export interface ModuleNodeInfo {
  id: string;
  instance_name: string;      // 用于生成引用的名称（如 AWS_network-policy_outpu）
  display_name?: string;      // 用于显示的友好名称（如 outpu）
  module_id?: number;
  module_source?: string;
  outputs?: ModuleOutput[];
}

// Module Output 定义
export interface ModuleOutput {
  name: string;
  type?: string;
  description?: string;
}

// 统一的引用源类型
interface ReferenceSource {
  id: string;
  type: 'module' | 'local';  // module = 本地资源, local = 远程数据
  name: string;              // 显示名称
  instanceName: string;      // 用于生成引用的名称
  description?: string;      // 描述
  moduleId?: number;         // Module ID（用于加载 outputs）
  outputs?: ModuleOutput[] | RemoteDataOutput[];  // 可用的 outputs
}

interface ModuleReferencePopoverProps {
  open: boolean;
  onClose: () => void;
  onSelect: (reference: string, sourceNodeId: string, outputName: string) => void;
  currentNodeId: string;  // 当前正在编辑的节点 ID
  nodes: ModuleNodeInfo[];  // 画布中所有的 Module 节点
  connectedNodeIds?: string[];  // 已连线的节点 ID 列表（只有这些节点可以被引用）
  children?: React.ReactNode;  // 触发元素
  position?: { x: number; y: number };
  remoteData?: RemoteDataNode[];  // 远程数据引用（新增）
}

const ModuleReferencePopover: React.FC<ModuleReferencePopoverProps> = ({
  open,
  onClose,
  onSelect,
  currentNodeId,
  nodes,
  connectedNodeIds,
  children,
  position,
  remoteData,
}) => {
  const [step, setStep] = useState<'select_source' | 'select_output'>('select_source');
  const [selectedSource, setSelectedSource] = useState<ReferenceSource | null>(null);
  const [searchText, setSearchText] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadedOutputs, setLoadedOutputs] = useState<ModuleOutput[]>([]);
  
  // 拖动相关状态
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const popoverRef = React.useRef<HTMLDivElement>(null);

  // 过滤掉当前节点，只显示已连线的 Module 节点
  const availableModules = useMemo(() => {
    // 如果提供了 connectedNodeIds，只显示已连线的节点
    if (connectedNodeIds && connectedNodeIds.length > 0) {
      return nodes.filter(n => 
        n.id !== currentNodeId && 
        n.instance_name && 
        connectedNodeIds.includes(n.id)
      );
    }
    // 如果没有提供 connectedNodeIds，显示所有其他节点（向后兼容）
    return nodes.filter(n => n.id !== currentNodeId && n.instance_name);
  }, [nodes, currentNodeId, connectedNodeIds]);

  // 合并所有引用源：module（本地资源）+ local（远程数据）
  const allSources = useMemo((): ReferenceSource[] => {
    const sources: ReferenceSource[] = [];
    
    // 添加本地资源（module 类型）
    availableModules.forEach(m => {
      sources.push({
        id: m.id,
        type: 'module',
        name: m.display_name || m.instance_name,
        instanceName: m.instance_name,
        description: m.module_source,
        moduleId: m.module_id,
        outputs: m.outputs,
      });
    });
    
    // 添加远程数据（local 类型）
    if (remoteData) {
      remoteData.forEach(rd => {
        sources.push({
          id: rd.remote_data_id,
          type: 'local',
          name: rd.data_name,
          instanceName: rd.data_name,
          description: rd.source_workspace_name || rd.source_workspace_id,
          outputs: rd.available_outputs,
        });
      });
    }
    
    return sources;
  }, [availableModules, remoteData]);

  // 搜索过滤
  const filteredSources = useMemo(() => {
    if (!searchText) return allSources;
    const query = searchText.toLowerCase();
    return allSources.filter(s => 
      s.name.toLowerCase().includes(query) ||
      s.instanceName.toLowerCase().includes(query) ||
      s.description?.toLowerCase().includes(query)
    );
  }, [allSources, searchText]);

  // 重置状态
  useEffect(() => {
    if (open) {
      setStep('select_source');
      setSelectedSource(null);
      setSearchText('');
      setDragOffset({ x: 0, y: 0 });
      setLoadedOutputs([]);
    }
  }, [open]);

  // 计算弹出框位置，确保不超出边界
  const getAdjustedPosition = () => {
    if (!position) return { x: 0, y: 0 };
    
    const popoverWidth = 320;
    const popoverHeight = 400;
    const padding = 16;
    
    let x = position.x + dragOffset.x;
    let y = position.y + dragOffset.y;
    
    // 检查右边界
    if (x + popoverWidth > window.innerWidth - padding) {
      x = window.innerWidth - popoverWidth - padding;
    }
    // 检查左边界
    if (x < padding) {
      x = padding;
    }
    // 检查下边界
    if (y + popoverHeight > window.innerHeight - padding) {
      y = window.innerHeight - popoverHeight - padding;
    }
    // 检查上边界
    if (y < padding) {
      y = padding;
    }
    
    return { x, y };
  };

  // 拖动开始
  const handleDragStart = (e: React.MouseEvent) => {
    e.preventDefault();
    setIsDragging(true);
    const startX = e.clientX;
    const startY = e.clientY;
    const startOffsetX = dragOffset.x;
    const startOffsetY = dragOffset.y;
    
    const handleMouseMove = (moveEvent: MouseEvent) => {
      setDragOffset({
        x: startOffsetX + moveEvent.clientX - startX,
        y: startOffsetY + moveEvent.clientY - startY,
      });
    };
    
    const handleMouseUp = () => {
      setIsDragging(false);
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
    
    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
  };

  // 选择引用源 - 动态加载 outputs
  const handleSelectSource = async (source: ReferenceSource) => {
    setSelectedSource(source);
    setStep('select_output');
    setSearchText('');
    setLoadedOutputs([]);
    
    // 如果是 module 类型且有 moduleId，从 Schema 中获取 outputs
    if (source.type === 'module' && source.moduleId) {
      setLoading(true);
      try {
        const schema = await schemaV2Service.getSchemaV2(source.moduleId);
        const openapiSchema = schema?.openapi_schema as any;
        const outputItems = openapiSchema?.['x-iac-platform']?.outputs?.items;
        
        if (outputItems && Array.isArray(outputItems)) {
          const outputs: ModuleOutput[] = outputItems.map((item: any) => ({
            name: item.name,
            type: item.type || 'string',
            description: item.description || '',
          }));
          setLoadedOutputs(outputs);
        } else {
          // 尝试从 components.schemas.ModuleOutput 获取（兼容旧格式）
          const schemas = openapiSchema?.components?.schemas as any;
          const outputSchema = schemas?.ModuleOutput?.properties;
          if (outputSchema) {
            const outputs: ModuleOutput[] = Object.entries(outputSchema).map(([name, prop]: [string, any]) => ({
              name,
              type: prop.type || 'string',
              description: prop.description || prop.title || '',
            }));
            setLoadedOutputs(outputs);
          }
        }
      } catch (error) {
        console.error(`[ModuleReferencePopover] Failed to load schema for module ${source.moduleId}:`, error);
      } finally {
        setLoading(false);
      }
    }
  };

  // 选择 Output
  const handleSelectOutput = (outputName: string) => {
    if (!selectedSource) return;
    
    let reference: string;
    if (selectedSource.type === 'module') {
      // 本地资源引用格式：module.{instance_name}.{output_name}
      reference = `module.${selectedSource.instanceName}.${outputName}`;
    } else {
      // 远程数据引用格式：local.{data_name}.{output_key}.value
      reference = `local.${selectedSource.instanceName}.${outputName}.value`;
    }
    
    onSelect(reference, selectedSource.id, outputName);
    onClose();
  };

  // 返回选择源
  const handleBack = () => {
    setStep('select_source');
    setSelectedSource(null);
    setSearchText('');
  };

  // 获取当前步骤标题
  const getStepTitle = () => {
    if (step === 'select_source') {
      return '选择引用源';
    }
    return '选择 Output';
  };

  // 渲染引用源列表
  const renderSourceList = () => (
    <div className={styles.listContainer}>
      <Input
        placeholder="搜索..."
        value={searchText}
        onChange={(e) => setSearchText(e.target.value)}
        allowClear
        size="small"
        className={styles.searchInput}
      />
      {filteredSources.length === 0 ? (
        <Empty 
          description="没有可用的引用源" 
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          className={styles.empty}
        />
      ) : (
        <List
          size="small"
          dataSource={filteredSources}
          renderItem={(source) => (
            <List.Item
              className={styles.listItem}
              onClick={() => handleSelectSource(source)}
            >
              <div className={styles.moduleItem}>
                {source.type === 'module' ? (
                  <AppstoreOutlined className={styles.moduleIcon} />
                ) : (
                  <CloudOutlined className={styles.moduleIcon} style={{ color: '#10b981' }} />
                )}
                <div className={styles.moduleInfo}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                    <Text strong className={styles.moduleName}>
                      {source.name}
                    </Text>
                    <Tag 
                      color={source.type === 'module' ? 'blue' : 'green'} 
                      style={{ fontSize: 10, padding: '0 4px', lineHeight: '16px', height: 16 }}
                    >
                      {source.type}
                    </Tag>
                  </div>
                  {source.description && (
                    <Text type="secondary" className={styles.moduleSource}>
                      {source.description.length > 40 
                        ? source.description.substring(0, 40) + '...'
                        : source.description}
                    </Text>
                  )}
                </div>
              </div>
            </List.Item>
          )}
        />
      )}
    </div>
  );

  // 渲染 Output 列表
  const renderOutputList = () => {
    if (!selectedSource) return null;

    // 获取 outputs：优先使用动态加载的，否则使用源自带的
    let outputs: Array<{ name: string; type?: string; description?: string }> = [];
    
    if (loadedOutputs.length > 0) {
      outputs = loadedOutputs;
    } else if (selectedSource.outputs) {
      if (selectedSource.type === 'module') {
        outputs = selectedSource.outputs as ModuleOutput[];
      } else {
        // 远程数据的 outputs 格式不同，需要转换
        outputs = (selectedSource.outputs as RemoteDataOutput[]).map(o => ({
          name: o.key,
          type: o.type,
          description: o.sensitive ? '(sensitive)' : undefined,
        }));
      }
    }

    // 搜索过滤
    const filteredOutputs = searchText
      ? outputs.filter(o => 
          o.name.toLowerCase().includes(searchText.toLowerCase()) ||
          o.description?.toLowerCase().includes(searchText.toLowerCase())
        )
      : outputs;

    return (
      <div className={styles.listContainer}>
        <div className={styles.header}>
          <a onClick={handleBack} className={styles.backLink}>← 返回</a>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <Text strong>{selectedSource.name}</Text>
            <Tag 
              color={selectedSource.type === 'module' ? 'blue' : 'green'} 
              style={{ fontSize: 10, padding: '0 4px', lineHeight: '16px', height: 16 }}
            >
              {selectedSource.type}
            </Tag>
          </div>
        </div>
        <Input
          placeholder="搜索 Output..."
          value={searchText}
          onChange={(e) => setSearchText(e.target.value)}
          allowClear
          size="small"
          className={styles.searchInput}
        />
        {loading ? (
          <div style={{ textAlign: 'center', padding: 20 }}>
            <Spin size="small" />
            <div style={{ marginTop: 8, fontSize: 12, color: '#999' }}>加载 Outputs...</div>
          </div>
        ) : filteredOutputs.length === 0 ? (
          <Empty 
            description="没有可用的 Outputs" 
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            className={styles.empty}
          />
        ) : (
          <List
            size="small"
            dataSource={filteredOutputs}
            renderItem={(output) => (
              <List.Item
                className={styles.listItem}
                onClick={() => handleSelectOutput(output.name)}
              >
                <div className={styles.outputItem}>
                  <ApiOutlined 
                    className={styles.outputIcon} 
                    style={{ color: selectedSource.type === 'module' ? '#52c41a' : '#10b981' }}
                  />
                  <div className={styles.outputInfo}>
                    <div className={styles.outputNameRow}>
                      <Text strong className={styles.outputName}>
                        {output.name}
                      </Text>
                      {output.type && (
                        <Tag color={selectedSource.type === 'module' ? 'blue' : 'green'} className={styles.outputType}>
                          {output.type}
                        </Tag>
                      )}
                    </div>
                    {output.description && (
                      <Text type="secondary" className={styles.outputDesc}>
                        {output.description}
                      </Text>
                    )}
                  </div>
                </div>
              </List.Item>
            )}
          />
        )}
      </div>
    );
  };

  if (!open) return null;

  const content = (
    <div className={styles.popoverContent}>
      <div className={styles.title}>
        {getStepTitle()}
      </div>
      <Spin spinning={loading}>
        {step === 'select_source' ? renderSourceList() : renderOutputList()}
      </Spin>
    </div>
  );

  // 使用绝对定位的弹出层
  if (position) {
    const adjustedPos = getAdjustedPosition();
    return (
      <div 
        className={styles.popoverOverlay}
        onClick={(e) => {
          if (e.target === e.currentTarget) onClose();
        }}
      >
        <div 
          ref={popoverRef}
          className={styles.popoverBox}
          style={{ 
            left: adjustedPos.x, 
            top: adjustedPos.y,
            cursor: isDragging ? 'grabbing' : 'default',
          }}
        >
          {/* 拖动手柄 */}
          <div 
            className={styles.dragHandle}
            onMouseDown={handleDragStart}
            style={{ cursor: isDragging ? 'grabbing' : 'grab' }}
          >
            <span className={styles.dragIcon}>⋮⋮</span>
            <span>{getStepTitle()}</span>
          </div>
          <div className={styles.popoverBody}>
            <Spin spinning={loading}>
              {step === 'select_source' ? renderSourceList() : renderOutputList()}
            </Spin>
          </div>
        </div>
      </div>
    );
  }

  // 使用 Popover 组件
  return (
    <Popover
      open={open}
      onOpenChange={(visible) => !visible && onClose()}
      content={content}
      trigger="click"
      placement="bottomLeft"
      overlayClassName={styles.popover}
    >
      {children}
    </Popover>
  );
};

export default ModuleReferencePopover;
export { ModuleReferencePopover };
