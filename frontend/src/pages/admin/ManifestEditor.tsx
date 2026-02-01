import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  ReactFlow,
  ReactFlowProvider,
  Controls,
  Background,
  MiniMap,
  addEdge,
  useNodesState,
  useEdgesState,
  BackgroundVariant,
  Panel,
  SmoothStepEdge,
  ConnectionMode,
  useReactFlow,
  MarkerType,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import {
  Button,
  Space,
  message,
  Spin,
  Typography,
  Drawer,
  Card,
  Input,
  Modal,
  Form,
  Divider,
  InputNumber,
  Switch,
  Select,
  List,
  Tag,
  Timeline,
  Tooltip,
} from 'antd';
import {
  SaveOutlined,
  CloudUploadOutlined,
  ExportOutlined,
  ArrowLeftOutlined,
  PlusOutlined,
  AppstoreOutlined,
  SearchOutlined,
  HistoryOutlined,
  RocketOutlined,
  BorderOutlined,
  FontSizeOutlined,
  EyeOutlined,
  CopyOutlined,
  UndoOutlined,
  RedoOutlined,
} from '@ant-design/icons';
import { moduleService } from '../../services/modules';
import type { Module } from '../../services/modules';
import { schemaV2Service, extractFieldsFromSchema, getWidgetType } from '../../services/schemaV2';
import type { SchemaV2, PropertySchema, FieldUIConfig } from '../../services/schemaV2';
import { useToast } from '../../contexts/ToastContext';
import type {
  Manifest,
  ManifestVersion,
  ManifestNode as ManifestNodeData,
  ManifestEdge as ManifestEdgeData,
  ManifestCanvasData,
  SaveManifestVersionRequest,
} from '../../services/manifestApi';
import {
  getManifest,
  listManifestVersions,
  saveManifestDraft,
  publishManifestVersion,
  exportManifestHCL,
  exportManifestZip,
  createManifestDeployment,
} from '../../services/manifestApi';
import ModuleNode from '../../components/ManifestEditor/ModuleNode';
import GroupNode from '../../components/ManifestEditor/GroupNode';
import AnnotationNode from '../../components/ManifestEditor/AnnotationNode';
import { ModuleFormRenderer } from '../../components/ModuleFormRenderer';
import DemoSelector from '../../components/DemoSelector';
import { AIConfigGenerator } from '../../components/OpenAPIFormRenderer/AIFormAssistant';
import ConfirmDialog from '../../components/ConfirmDialog';
import styles from './ManifestEditor.module.css';

const { Title, Text } = Typography;

// 根据 Schema 属性渲染表单字段
const renderFormField = (property: PropertySchema, uiConfig: FieldUIConfig) => {
  const widgetType = getWidgetType(property, uiConfig);
  
  switch (widgetType) {
    case 'switch':
      return <Switch />;
    case 'number':
      return (
        <InputNumber
          style={{ width: '100%' }}
          min={property.minimum}
          max={property.maximum}
          placeholder={uiConfig.placeholder}
        />
      );
    case 'select':
      return (
        <Select
          placeholder={uiConfig.placeholder || '请选择'}
          allowClear
          options={property.enum?.map(v => ({ label: v, value: v }))}
        />
      );
    case 'textarea':
      return (
        <Input.TextArea
          rows={3}
          placeholder={uiConfig.placeholder}
        />
      );
    default:
      return (
        <Input
          placeholder={uiConfig.placeholder}
          type={property['x-sensitive'] ? 'password' : 'text'}
        />
      );
  }
};

// 自定义节点类型
const nodeTypes = {
  module: ModuleNode,
  group: GroupNode,
  annotation: AnnotationNode,
};

// 自定义边类型
const edgeTypes = {
  smoothstep: SmoothStepEdge,
};

// 修复配置中的数组值（将字符串格式转换为数组格式）
const fixArrayValues = (config: Record<string, unknown>): Record<string, unknown> => {
  if (!config || typeof config !== 'object') return config;
  
  const fixed: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(config)) {
    if (typeof value === 'string' && value.includes('\n')) {
      // 如果是包含换行符的字符串，转换为数组
      fixed[key] = value.split('\n').map(line => line.trim()).filter(line => line.length > 0);
    } else {
      fixed[key] = value;
    }
  }
  return fixed;
};

// 将 ManifestNode 转换为 React Flow Node
const convertToFlowNodes = (nodes: ManifestNodeData[]): any[] => {
  return nodes.map((node) => {
    const nodeType = (node as any).type || 'module';
    
    if (nodeType === 'group') {
      return {
        id: node.id,
        type: 'group',
        position: node.position,
        style: (node as any).style || { width: 200, height: 150, zIndex: -1 },
        data: {
          type: 'group',
          label: node.config?.label || node.instance_name || '分组',
          color: node.config?.color || '#6495ED',
        },
      };
    } else if (nodeType === 'annotation') {
      return {
        id: node.id,
        type: 'annotation',
        position: node.position,
        data: {
          type: 'annotation',
          text: node.config?.text || node.instance_name || '双击编辑文字',
          fontSize: node.config?.fontSize || 12,
          color: node.config?.color || '#666',
        },
      };
    } else {
      return {
        id: node.id,
        type: 'module',
        position: node.position,
        data: {
          ...node,
          label: node.instance_name || node.resource_name,
          // 修复配置中的数组值
          config: fixArrayValues(node.config || {}),
        },
      };
    }
  });
};

// 将 ManifestEdge 转换为 React Flow Edge
const convertToFlowEdges = (edges: ManifestEdgeData[]): any[] => {
  console.log('🔄 convertToFlowEdges input:', edges);
  const result = edges.map((edge) => {
    // dependency: 蓝色, variable_binding: 绿色
    const strokeColor = edge.type === 'dependency' ? '#1890ff' : '#52c41a';
    const flowEdge = {
      id: edge.id,
      source: edge.source.node_id,
      target: edge.target.node_id,
      // 恢复保存的连接点位置
      sourceHandle: edge.source.port_id || undefined,
      targetHandle: edge.target.port_id || undefined,
      type: 'default',  // 使用默认边类型
      animated: false,  // 不使用动画，避免虚线滚动效果
      style: {
        stroke: strokeColor,
        strokeWidth: edge.type === 'variable_binding' ? 2 : 1,  // variable_binding 稍粗一点以区分
      },
      // 选中时的样式
      selected: false,
      selectable: true,
      markerEnd: {
        type: MarkerType.ArrowClosed,
        color: strokeColor,
        width: 15,
        height: 15,
      },
      zIndex: 1000,  // 确保边在节点上方
      data: { ...edge },
    };
    console.log('  📍 Converted edge:', flowEdge);
    return flowEdge;
  });
  console.log('🔄 convertToFlowEdges output:', result);
  return result;
};

// 将 React Flow Node 转换回 ManifestNode
const convertFromFlowNodes = (nodes: any[]): ManifestNodeData[] => {
  return nodes.map((node) => {
    const nodeType = node.type || node.data?.type || 'module';
    
    // 基础节点数据
    const baseNode: any = {
      id: node.id,
      type: nodeType,
      position: node.position,
    };
    
    // 根据节点类型添加不同的数据
    if (nodeType === 'group') {
      return {
        ...baseNode,
        instance_name: node.data?.label || '分组',
        resource_name: '',
        is_linked: false,
        link_status: 'unlinked',
        config: {
          label: node.data?.label,
          color: node.data?.color,
        },
        config_complete: true,
        ports: [],
        // 保存尺寸
        style: node.style,
      };
    } else if (nodeType === 'annotation') {
      return {
        ...baseNode,
        instance_name: node.data?.text || '文字说明',
        resource_name: '',
        is_linked: false,
        link_status: 'unlinked',
        config: {
          text: node.data?.text,
          fontSize: node.data?.fontSize,
          color: node.data?.color,
        },
        config_complete: true,
        ports: [],
      };
    } else {
      // module 类型
      return {
        ...baseNode,
        module_id: node.data?.module_id,
        is_linked: node.data?.is_linked || false,
        link_status: node.data?.link_status || 'unlinked',
        module_source: node.data?.module_source,
        module_version: node.data?.module_version,
        instance_name: node.data?.instance_name || node.data?.label || '',
        resource_name: node.data?.resource_name || '',
        config: node.data?.config || {},
        config_complete: node.data?.config_complete || false,
        ports: node.data?.ports || [],
      };
    }
  });
};

// 将 React Flow Edge 转换回 ManifestEdge
const convertFromFlowEdges = (edges: any[]): ManifestEdgeData[] => {
  return edges.map((edge) => ({
    id: edge.id,
    type: edge.data?.type || 'dependency',
    source: {
      node_id: edge.source,
      port_id: edge.sourceHandle || undefined,
    },
    target: {
      node_id: edge.target,
      port_id: edge.targetHandle || undefined,
    },
    expression: edge.data?.expression,
  }));
};

// 内部组件，使用 useReactFlow hook
const ManifestEditorInner: React.FC = () => {
  const { id: manifestId } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const toast = useToast();
  const reactFlowInstance = useReactFlow();

  // 状态
  const [manifest, setManifest] = useState<Manifest | null>(null);
  const [draftVersion, setDraftVersion] = useState<ManifestVersion | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [publishing, setPublishing] = useState(false);
  
  // Module 库状态
  const [modules, setModules] = useState<Module[]>([]);
  const [modulesLoading, setModulesLoading] = useState(false);
  const [moduleSearchText, setModuleSearchText] = useState('');

  // React Flow 状态
  const [nodes, setNodes, onNodesChange] = useNodesState<any>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<any>([]);
  const [viewport, setViewport] = useState({ x: 0, y: 0, zoom: 1 });

  // UI 状态 - 从 localStorage 读取用户偏好
  const [moduleLibraryOpen, setModuleLibraryOpen] = useState(() => {
    const saved = localStorage.getItem('manifest_moduleLibraryOpen');
    return saved !== null ? saved === 'true' : true;
  });
  const [propertiesPanelOpen, setPropertiesPanelOpen] = useState(() => {
    const saved = localStorage.getItem('manifest_propertiesPanelOpen');
    return saved !== null ? saved === 'true' : true;
  });
  const [selectedNode, setSelectedNode] = useState<any>(null);
  const [publishModalOpen, setPublishModalOpen] = useState(false);
  const [publishForm] = Form.useForm();
  const [versionsDrawerOpen, setVersionsDrawerOpen] = useState(false);
  const [versions, setVersions] = useState<ManifestVersion[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [deployModalOpen, setDeployModalOpen] = useState(false);
  const [deployForm] = Form.useForm();
  const [deploying, setDeploying] = useState(false);
  const [workspaces, setWorkspaces] = useState<any[]>([]);
  const [workspacesLoading, setWorkspacesLoading] = useState(false);
  
  // 版本详情状态
  const [versionDetailModalOpen, setVersionDetailModalOpen] = useState(false);
  const [selectedVersion, setSelectedVersion] = useState<ManifestVersion | null>(null);
  
  // HCL 预览状态
  const [previewModalOpen, setPreviewModalOpen] = useState(false);
  const [previewHCL, setPreviewHCL] = useState('');
  const [previewLoading, setPreviewLoading] = useState(false);
  
  // 节点配置状态
  const [nodeSchema, setNodeSchema] = useState<SchemaV2 | null>(null);
  const [schemaLoading, setSchemaLoading] = useState(false);
  const [nodeConfigForm] = Form.useForm();
  
  // Demo 选择确认对话框状态
  const [showDemoConfirmDialog, setShowDemoConfirmDialog] = useState(false);
  const [pendingDemoData, setPendingDemoData] = useState<any>(null);
  const [pendingDemoName, setPendingDemoName] = useState<string>('');
  
  // 面板宽度状态 - 从 localStorage 读取
  const [moduleLibraryWidth, setModuleLibraryWidth] = useState(() => {
    const saved = localStorage.getItem('manifest_moduleLibraryWidth');
    return saved ? parseInt(saved, 10) : 200;
  });
  const [propertiesPanelWidth, setPropertiesPanelWidth] = useState(() => {
    const saved = localStorage.getItem('manifest_propertiesPanelWidth');
    return saved ? parseInt(saved, 10) : 250;
  });

  // 保存面板状态到 localStorage
  useEffect(() => {
    localStorage.setItem('manifest_moduleLibraryOpen', String(moduleLibraryOpen));
  }, [moduleLibraryOpen]);

  useEffect(() => {
    localStorage.setItem('manifest_propertiesPanelOpen', String(propertiesPanelOpen));
  }, [propertiesPanelOpen]);

  useEffect(() => {
    localStorage.setItem('manifest_moduleLibraryWidth', String(moduleLibraryWidth));
  }, [moduleLibraryWidth]);

  useEffect(() => {
    localStorage.setItem('manifest_propertiesPanelWidth', String(propertiesPanelWidth));
  }, [propertiesPanelWidth]);

  // 自动保存定时器
  const autoSaveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const hasChangesRef = useRef(false);

  // 剪贴板状态
  const clipboardRef = useRef<any>(null);

  // 撤销/重做历史记录
  const [history, setHistory] = useState<Array<{ nodes: any[]; edges: any[] }>>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const isUndoRedoRef = useRef(false);  // 标记是否正在执行撤销/重做操作
  const maxHistoryLength = 50;  // 最大历史记录数
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // 保存当前状态到历史记录（使用 ref 获取最新状态）
  const nodesRef = useRef(nodes);
  const edgesRef = useRef(edges);
  const historyIndexRef = useRef(historyIndex);
  
  // 同步 ref
  useEffect(() => {
    nodesRef.current = nodes;
  }, [nodes]);
  
  useEffect(() => {
    edgesRef.current = edges;
  }, [edges]);
  
  useEffect(() => {
    historyIndexRef.current = historyIndex;
  }, [historyIndex]);

  // 保存当前状态到历史记录（防抖，避免频繁保存）
  const saveToHistory = useCallback(() => {
    if (isUndoRedoRef.current) {
      isUndoRedoRef.current = false;
      return;
    }
    
    // 清除之前的定时器
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current);
    }
    
    // 延迟 300ms 保存，避免频繁操作时产生过多历史记录
    saveTimeoutRef.current = setTimeout(() => {
      const currentNodes = nodesRef.current;
      const currentEdges = edgesRef.current;
      const currentHistoryIndex = historyIndexRef.current;
      
      setHistory((prev) => {
        // 如果当前不在历史末尾，删除后面的记录
        const newHistory = prev.slice(0, currentHistoryIndex + 1);
        // 添加新状态
        newHistory.push({
          nodes: JSON.parse(JSON.stringify(currentNodes)),
          edges: JSON.parse(JSON.stringify(currentEdges)),
        });
        // 限制历史记录长度
        if (newHistory.length > maxHistoryLength) {
          newHistory.shift();
          return newHistory;
        }
        return newHistory;
      });
      setHistoryIndex((prev) => Math.min(prev + 1, maxHistoryLength - 1));
    }, 300);
  }, []);

  // 撤销
  const handleUndo = useCallback(() => {
    if (historyIndex <= 0) {
      toast.info('没有可撤销的操作');
      return;
    }
    
    isUndoRedoRef.current = true;
    const newIndex = historyIndex - 1;
    const state = history[newIndex];
    if (state) {
      setNodes(state.nodes);
      setEdges(state.edges);
      setHistoryIndex(newIndex);
      hasChangesRef.current = true;
    }
  }, [history, historyIndex, setNodes, setEdges]);

  // 重做
  const handleRedo = useCallback(() => {
    if (historyIndex >= history.length - 1) {
      toast.info('没有可重做的操作');
      return;
    }
    
    isUndoRedoRef.current = true;
    const newIndex = historyIndex + 1;
    const state = history[newIndex];
    if (state) {
      setNodes(state.nodes);
      setEdges(state.edges);
      setHistoryIndex(newIndex);
      hasChangesRef.current = true;
    }
  }, [history, historyIndex, setNodes, setEdges]);

  // 是否可以撤销/重做
  const canUndo = historyIndex > 0;
  const canRedo = historyIndex < history.length - 1;

  // 页面刷新提示
  useEffect(() => {
    const handleBeforeUnload = (e: BeforeUnloadEvent) => {
      if (hasChangesRef.current) {
        e.preventDefault();
        e.returnValue = '您有未保存的更改，确定要离开吗？';
        return e.returnValue;
      }
    };

    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
    };
  }, []);

  // 键盘快捷键：复制粘贴、撤销重做
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // 如果焦点在输入框中，不处理（除了撤销/重做）
      const isInInput = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement;

      // Ctrl+Z 或 Cmd+Z 撤销
      if ((e.ctrlKey || e.metaKey) && e.key === 'z' && !e.shiftKey) {
        e.preventDefault();
        handleUndo();
        return;
      }

      // Ctrl+Shift+Z 或 Cmd+Shift+Z 重做
      if ((e.ctrlKey || e.metaKey) && e.key === 'z' && e.shiftKey) {
        e.preventDefault();
        handleRedo();
        return;
      }

      // Ctrl+Y 或 Cmd+Y 重做（Windows 风格）
      if ((e.ctrlKey || e.metaKey) && e.key === 'y') {
        e.preventDefault();
        handleRedo();
        return;
      }

      // 以下快捷键在输入框中不处理
      if (isInInput) {
        return;
      }

      // Ctrl+C 或 Cmd+C 复制
      if ((e.ctrlKey || e.metaKey) && e.key === 'c') {
        if (selectedNode) {
          clipboardRef.current = {
            ...selectedNode,
            id: null, // 清除 ID，粘贴时生成新的
          };
          toast.success('节点已复制到剪贴板');
        }
      }

      // Ctrl+V 或 Cmd+V 粘贴
      if ((e.ctrlKey || e.metaKey) && e.key === 'v') {
        if (clipboardRef.current) {
          const newNode = {
            ...clipboardRef.current,
            id: `node-${Date.now()}`,
            position: {
              x: (clipboardRef.current.position?.x || 100) + 50,
              y: (clipboardRef.current.position?.y || 100) + 50,
            },
            data: {
              ...clipboardRef.current.data,
              instance_name: `${clipboardRef.current.data?.instance_name || 'module'}_copy`,
              label: `${clipboardRef.current.data?.label || 'module'}_copy`,
            },
          };
          setNodes((nds: any[]) => [...nds, newNode]);
          markChanges();
          toast.success('节点已粘贴');
        }
      }

      // Delete 或 Backspace 删除
      if (e.key === 'Delete' || e.key === 'Backspace') {
        if (selectedNode) {
          setNodes((nds: any[]) => nds.filter((n) => n.id !== selectedNode.id));
          setEdges((eds: any[]) =>
            eds.filter((e) => e.source !== selectedNode.id && e.target !== selectedNode.id)
          );
          setSelectedNode(null);
          setNodeSchema(null);
          markChanges();
          toast.success('节点已删除');
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [selectedNode, setNodes, setEdges, handleUndo, handleRedo]);

  // 从 manifest 获取组织 ID
  const orgId = manifest?.organization_id?.toString() || '1';

  // 加载 Manifest 数据
  const loadManifest = async () => {
    console.log('🚀 loadManifest called, manifestId:', manifestId);
    if (!manifestId) return;

    setLoading(true);
    try {
      const manifestData = await getManifest(orgId, manifestId);
      console.log('📋 Manifest loaded:', manifestData.name);
      setManifest(manifestData);

      // 获取草稿版本
      const versionsResponse = await listManifestVersions(orgId, manifestId);
      console.log('📚 Versions loaded:', versionsResponse.items.length);
      const draft = versionsResponse.items.find((v) => v.is_draft);

      if (draft) {
        console.log('📝 Draft found:', draft.id, 'nodes:', draft.nodes?.length, 'edges:', draft.edges?.length);
        setDraftVersion(draft);
        // 转换并设置节点和边
        const flowNodes = convertToFlowNodes(draft.nodes || []);
        const flowEdges = convertToFlowEdges(draft.edges || []);
        
        // 调试：打印节点和边的 ID
        console.log('📦 Flow Nodes:', flowNodes.map(n => ({ id: n.id, type: n.type, position: n.position })));
        console.log('🔗 Flow Edges:', flowEdges.map(e => ({ id: e.id, source: e.source, target: e.target, sourceHandle: e.sourceHandle, targetHandle: e.targetHandle })));
        
        // 检查边引用的节点是否存在
        const nodeIds = new Set(flowNodes.map(n => n.id));
        flowEdges.forEach(e => {
          if (!nodeIds.has(e.source)) {
            console.error(`❌ Edge ${e.id} references non-existent source node: ${e.source}`);
          }
          if (!nodeIds.has(e.target)) {
            console.error(`❌ Edge ${e.id} references non-existent target node: ${e.target}`);
          }
        });
        
        // 先设置节点
        setNodes(flowNodes);
        
        // 同时设置节点和边
        setEdges(flowEdges);
        console.log('✅ Nodes and edges set');
        
        // 初始化历史记录
        setHistory([{
          nodes: JSON.parse(JSON.stringify(flowNodes)),
          edges: JSON.parse(JSON.stringify(flowEdges)),
        }]);
        setHistoryIndex(0);
        if (draft.canvas_data) {
          setViewport({
            x: draft.canvas_data.viewport?.x || 0,
            y: draft.canvas_data.viewport?.y || 0,
            zoom: draft.canvas_data.zoom || 1,
          });
        }
      }
    } catch (error: any) {
      toast.error('加载 Manifest 失败: ' + (error.message || '未知错误'));
    } finally {
      setLoading(false);
    }
  };

  // 加载版本历史
  const loadVersions = async () => {
    if (!manifestId) return;
    setVersionsLoading(true);
    try {
      const response = await listManifestVersions(orgId, manifestId);
      setVersions(response.items || []);
    } catch (error: any) {
      console.error('加载版本历史失败:', error);
    } finally {
      setVersionsLoading(false);
    }
  };

  // 加载 Module 库
  const loadModules = async () => {
    setModulesLoading(true);
    try {
      const response = await moduleService.getModules();
      // API 返回格式: { code: 200, data: { items: [...] } }
      // axios 拦截器返回 response.data，所以这里是 { data: { items: [...] } }
      let moduleList: Module[] = [];
      const resp = response as any;
      if (Array.isArray(resp)) {
        moduleList = resp;
      } else if (resp?.data?.items) {
        // { data: { items: [...] } }
        moduleList = resp.data.items;
      } else if (resp?.items) {
        // { items: [...] }
        moduleList = resp.items;
      } else if (resp?.data && Array.isArray(resp.data)) {
        // { data: [...] }
        moduleList = resp.data;
      }
      console.log('加载 Module 库:', moduleList.length, '个', resp);
      setModules(moduleList);
    } catch (error: any) {
      console.error('加载 Module 库失败:', error);
      setModules([]);
    } finally {
      setModulesLoading(false);
    }
  };

  // 加载 Workspace 列表
  const loadWorkspaces = async () => {
    setWorkspacesLoading(true);
    try {
      const response = await fetch('/api/v1/workspaces', {
        headers: {
          'Authorization': `Bearer ${localStorage.getItem('token')}`,
        },
      });
      if (response.ok) {
        const data = await response.json();
        // 兼容不同的响应格式
        const wsList = Array.isArray(data) ? data : (data.data || data.items || []);
        setWorkspaces(wsList);
      }
    } catch (error: any) {
      console.error('加载 Workspace 列表失败:', error);
      setWorkspaces([]);
    } finally {
      setWorkspacesLoading(false);
    }
  };

  useEffect(() => {
    loadManifest();
    loadModules();
    loadWorkspaces();
  }, [manifestId]);

  // 自动保存（防抖 2 秒）
  useEffect(() => {
    // 只有在有变更且已加载完成时才自动保存
    if (!hasChangesRef.current || loading || !manifestId) return;
    
    if (autoSaveTimerRef.current) {
      clearTimeout(autoSaveTimerRef.current);
    }
    
    autoSaveTimerRef.current = setTimeout(() => {
      handleSave(true);
    }, 2000); // 2 秒防抖自动保存

    return () => {
      if (autoSaveTimerRef.current) {
        clearTimeout(autoSaveTimerRef.current);
      }
    };
  }, [nodes, edges, loading, manifestId]);

  // 标记有变更并触发自动保存
  const markChanges = useCallback(() => {
    hasChangesRef.current = true;
    // 保存到历史记录
    saveToHistory();
    // 触发 useEffect 重新执行
    setNodes((nds) => [...nds]);
  }, [saveToHistory, setNodes]);

  // 处理节点变更
  const handleNodesChange = useCallback(
    (changes: any) => {
      onNodesChange(changes);
      markChanges();
    },
    [onNodesChange]
  );

  // 处理边变更
  const handleEdgesChange = useCallback(
    (changes: any) => {
      onEdgesChange(changes);
      markChanges();
    },
    [onEdgesChange]
  );

  // 处理连接（手动拖拽连线）
  const handleConnect = useCallback(
    (connection: any) => {
      console.log('🔗 handleConnect called:', connection);
      
      // 使用 setEdges 的函数形式来获取最新的 edges 状态
      setEdges((currentEdges: any[]) => {
        console.log('📊 Current edges:', currentEdges.length);
        
        // 检查两个节点之间是否已有连线
        const existingEdge = currentEdges.find((e: any) => 
          e.source === connection.source && e.target === connection.target
        );
        
        if (existingEdge) {
          // 两节点间已有边，不创建新边（用户可以通过表单中的 "/" 来添加更多引用）
          console.log('ℹ️ Edge already exists:', existingEdge.id);
          return currentEdges;  // 返回原数组，不做修改
        }
        
        // 创建新的依赖边（手动连线默认为 dependency 类型）
        const newEdge = {
          ...connection,
          id: `edge-${Date.now()}`,
          // 不指定 type，使用默认边类型
          style: { stroke: '#1890ff', strokeWidth: 1 },
          markerEnd: {
            type: MarkerType.ArrowClosed,
            color: '#1890ff',
            width: 15,
            height: 15,
          },
          data: { type: 'dependency' },
        };
        console.log('✅ New edge created:', newEdge.id);
        return addEdge(newEdge, currentEdges);
      });
      markChanges();
    },
    [setEdges, markChanges]
  );

  // 处理节点选择
  const handleNodeClick = useCallback(async (_: React.MouseEvent, node: any) => {
    setSelectedNode(node);
    setPropertiesPanelOpen(true);
    setNodeSchema(null);
    
    // 如果节点关联了 Module，加载其 Schema
    if (node.data?.module_id) {
      setSchemaLoading(true);
      try {
        const schema = await schemaV2Service.getSchemaV2(node.data.module_id);
        setNodeSchema(schema);
        // 设置表单初始值
        if (node.data?.config) {
          nodeConfigForm.setFieldsValue(node.data.config);
        }
      } catch (error: any) {
        console.error('加载 Schema 失败:', error);
      } finally {
        setSchemaLoading(false);
      }
    }
  }, [nodeConfigForm]);

  // 清理节点配置：移除 Schema 中不存在的字段
  const cleanNodeConfig = async (node: any): Promise<any> => {
    if (node.type !== 'module' || !node.data?.module_id) {
      return node;
    }

    try {
      // 获取 Schema
      const schema = await schemaV2Service.getSchemaV2(node.data.module_id);
      if (!schema?.openapi_schema?.components?.schemas?.ModuleInput?.properties) {
        return node;
      }

      const schemaProperties = schema.openapi_schema.components.schemas.ModuleInput.properties;
      const validKeys = Object.keys(schemaProperties);
      const currentConfig = node.data?.config || {};

      // 过滤掉 Schema 中不存在的字段
      const cleanedConfig: Record<string, unknown> = {};
      for (const key of validKeys) {
        if (key in currentConfig) {
          cleanedConfig[key] = currentConfig[key];
        }
      }

      return {
        ...node,
        data: {
          ...node.data,
          config: cleanedConfig,
        },
      };
    } catch (error) {
      console.warn('清理节点配置失败:', error);
      return node;
    }
  };

  // 保存草稿
  const handleSave = async (isAutoSave = false) => {
    if (!manifestId) return;

    setSaving(true);
    try {
      const canvasData: ManifestCanvasData = {
        viewport: { x: viewport.x, y: viewport.y },
        zoom: viewport.zoom,
      };

      // 清理节点配置（移除 Schema 中不存在的字段）
      const cleanedNodes = await Promise.all(nodes.map(cleanNodeConfig));
      
      // 更新本地节点状态
      setNodes(cleanedNodes);

      const data: SaveManifestVersionRequest = {
        canvas_data: canvasData,
        nodes: convertFromFlowNodes(cleanedNodes),
        edges: convertFromFlowEdges(edges),
        variables: draftVersion?.variables || [],
      };

      const savedVersion = await saveManifestDraft(orgId, manifestId, data);
      setDraftVersion(savedVersion);
      hasChangesRef.current = false;

      if (!isAutoSave) {
        toast.success('保存成功');
      } else {
        console.log('自动保存成功');
      }
    } catch (error: any) {
      message.error('保存失败: ' + (error.message || '未知错误'));
    } finally {
      setSaving(false);
    }
  };

  // 发布版本
  const handlePublish = async (values: { version: string }) => {
    if (!manifestId) return;

    // 先保存
    await handleSave();

    setPublishing(true);
    try {
      await publishManifestVersion(orgId, manifestId, { version: values.version });
      message.success('发布成功');
      setPublishModalOpen(false);
      publishForm.resetFields();
      loadManifest(); // 重新加载
    } catch (error: any) {
      message.error('发布失败: ' + (error.message || '未知错误'));
    } finally {
      setPublishing(false);
    }
  };

  // 预览 HCL
  const handlePreviewHCL = async () => {
    if (!manifestId) return;
    
    setPreviewLoading(true);
    setPreviewModalOpen(true);
    try {
      const hclContent = await exportManifestHCL(orgId, manifestId);
      setPreviewHCL(hclContent);
    } catch (error: any) {
      toast.error('预览失败: ' + (error.message || '未知错误'));
      setPreviewHCL('// 加载失败');
    } finally {
      setPreviewLoading(false);
    }
  };

  // 导出 ZIP (包含 manifest.json 和 .tf 文件)
  const handleExportHCL = async () => {
    if (!manifestId) return;
    
    try {
      const blob = await exportManifestZip(orgId, manifestId);
      
      // 创建下载链接
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${manifest?.name || 'manifest'}.zip`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      window.URL.revokeObjectURL(url);
      
      toast.success('导出成功 (ZIP 包含 manifest.json 和 .tf 文件)');
    } catch (error: any) {
      toast.error('导出失败: ' + (error.message || '未知错误'));
    }
  };

  // 复制 HCL 到剪贴板
  const handleCopyHCL = () => {
    navigator.clipboard.writeText(previewHCL).then(() => {
      toast.success('已复制到剪贴板');
    }).catch(() => {
      toast.error('复制失败');
    });
  };

  // 添加背景墙
  const handleAddGroup = () => {
    const newNode = {
      id: `group-${Date.now()}`,
      type: 'group',
      position: { x: 100, y: 100 },
      style: { width: 200, height: 150, zIndex: -1 },
      data: {
        type: 'group',
        label: '分组',
        color: 'rgba(100, 149, 237, 0.2)',
      },
    };
    setNodes((nds: any[]) => [...nds, newNode]);
    markChanges();
  };

  // 添加文字说明
  const handleAddAnnotation = () => {
    const newNode = {
      id: `annotation-${Date.now()}`,
      type: 'annotation',
      position: { x: 200, y: 50 },
      data: {
        type: 'annotation',
        text: '双击编辑文字',
        fontSize: 12,
        color: '#666',
      },
    };
    setNodes((nds: any[]) => [...nds, newNode]);
    markChanges();
  };

  // 获取画布中心位置
  const getCanvasCenter = () => {
    // 根据当前 viewport 计算画布中心
    // 假设画布可视区域大约 800x600
    const centerX = (-viewport.x + 400) / viewport.zoom;
    const centerY = (-viewport.y + 300) / viewport.zoom;
    return { x: centerX, y: centerY };
  };

  // 添加新节点（放在画布中心）
  const handleAddNode = (moduleData?: any, position?: { x: number; y: number }) => {
    const pos = position || getCanvasCenter();
    const newNode = {
      id: `node-${Date.now()}`,
      type: 'module',
      position: pos,
      data: {
        type: 'module',
        instance_name: moduleData?.name || `module_${nodes.length + 1}`,
        resource_name: moduleData?.name || 'New Module',
        is_linked: !!moduleData,
        link_status: moduleData ? 'linked' : 'unlinked',
        module_id: moduleData?.id,
        module_source: moduleData?.source,
        module_version: moduleData?.version,
        config: {},
        config_complete: false,
        ports: [],
        label: moduleData?.name || `module_${nodes.length + 1}`,
      },
    };
    setNodes((nds: any[]) => [...nds, newNode]);
    markChanges();
    // 不关闭 Module 库，方便继续添加
  };

  // 处理拖拽开始
  const handleDragStart = (event: React.DragEvent, moduleData: any) => {
    event.dataTransfer.setData('application/json', JSON.stringify(moduleData));
    event.dataTransfer.effectAllowed = 'move';
  };

  // 处理拖拽放置
  const handleDrop = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      const data = event.dataTransfer.getData('application/json');
      if (!data) return;

      try {
        const moduleData = JSON.parse(data);
        // 获取放置位置（相对于画布）
        const reactFlowBounds = event.currentTarget.getBoundingClientRect();
        const position = {
          x: (event.clientX - reactFlowBounds.left - viewport.x) / viewport.zoom,
          y: (event.clientY - reactFlowBounds.top - viewport.y) / viewport.zoom,
        };
        handleAddNode(moduleData, position);
      } catch (e) {
        console.error('拖拽数据解析失败:', e);
      }
    },
    [viewport]
  );

  const handleDragOver = (event: React.DragEvent) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  };

  if (loading) {
    return (
      <div className={styles.loadingContainer}>
        <Spin size="large" />
        <div style={{ marginTop: 16 }}>加载中...</div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {/* 顶部工具栏 */}
      <div className={styles.toolbar}>
        <Space>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/admin/manifests')}>
            返回
          </Button>
          <Divider type="vertical" />
          <Title level={5} style={{ margin: 0 }}>
            {manifest?.name || 'Manifest'}
          </Title>
          <Text type="secondary">
            {draftVersion?.version || 'draft'}
            {hasChangesRef.current && ' (未保存)'}
          </Text>
        </Space>
        <Space>
          <Button 
            icon={<AppstoreOutlined />} 
            onClick={() => setModuleLibraryOpen(!moduleLibraryOpen)}
            type={moduleLibraryOpen ? 'primary' : 'default'}
          >
            Module 库
          </Button>
          <Button icon={<BorderOutlined />} onClick={() => handleAddGroup()}>
            背景墙
          </Button>
          <Button icon={<FontSizeOutlined />} onClick={() => handleAddAnnotation()}>
            文字说明
          </Button>
          <Divider type="vertical" />
          <Tooltip title="撤销 (⌘Z)">
            <Button 
              icon={<UndoOutlined />} 
              onClick={handleUndo}
              disabled={!canUndo}
            />
          </Tooltip>
          <Tooltip title="重做 (⌘⇧Z)">
            <Button 
              icon={<RedoOutlined />} 
              onClick={handleRedo}
              disabled={!canRedo}
            />
          </Tooltip>
          <Divider type="vertical" />
          <Button icon={<SaveOutlined />} onClick={() => handleSave()} loading={saving}>
            保存草稿
          </Button>
          <Button
            type="primary"
            icon={<CloudUploadOutlined />}
            onClick={() => setPublishModalOpen(true)}
          >
            发布版本
          </Button>
          <Button icon={<HistoryOutlined />} onClick={() => {
            loadVersions();
            setVersionsDrawerOpen(true);
          }}>
            版本历史
          </Button>
          <Button icon={<RocketOutlined />} onClick={() => navigate(`/admin/manifests/${manifestId}/deploy`)}>
            部署
          </Button>
          <Button icon={<EyeOutlined />} onClick={handlePreviewHCL}>
            预览 HCL
          </Button>
          <Button icon={<ExportOutlined />} onClick={handleExportHCL}>
            导出
          </Button>
        </Space>
      </div>

      {/* 主编辑区域 */}
      <div className={styles.editorContainer}>
        {/* 左侧 Module 库面板 */}
        {moduleLibraryOpen && (
          <div className={styles.moduleLibraryPanel} style={{ width: moduleLibraryWidth }}>
            <div className={styles.moduleLibraryHeader}>
              <Text strong>Module 库</Text>
              <Button 
                type="text" 
                size="small" 
                onClick={() => setModuleLibraryOpen(false)}
              >
                ✕
              </Button>
            </div>
            <Input.Search 
              placeholder="搜索 Module..." 
              style={{ marginBottom: 8, padding: '0 8px' }}
              value={moduleSearchText}
              onChange={(e) => setModuleSearchText(e.target.value)}
              allowClear
            />
            <Spin spinning={modulesLoading}>
              <div className={styles.moduleLibraryList}>
                {(Array.isArray(modules) ? modules : [])
                  .filter(m => 
                    !moduleSearchText || 
                    m.name.toLowerCase().includes(moduleSearchText.toLowerCase()) ||
                    m.description?.toLowerCase().includes(moduleSearchText.toLowerCase())
                  )
                  .map((module) => (
                    <div
                      key={module.id}
                      className={styles.moduleLibraryItem}
                      draggable
                      onDragStart={(e) => handleDragStart(e, {
                        id: module.id,
                        name: module.name,
                        source: module.module_source || module.source,
                        version: module.version,
                        description: module.description,
                      })}
                      onClick={() => handleAddNode({
                        id: module.id,
                        name: module.name,
                        source: module.module_source || module.source,
                        version: module.version,
                        description: module.description,
                      })}
                    >
                      <Text strong style={{ fontSize: 12 }}>{module.name}</Text>
                      <Text type="secondary" style={{ fontSize: 10, display: 'block' }}>
                        {(module.module_source || module.source || '').substring(0, 30)}
                      </Text>
                    </div>
                  ))}
                {modules.length === 0 && !modulesLoading && (
                  <Text type="secondary" style={{ padding: 8 }}>暂无可用的 Module</Text>
                )}
              </div>
            </Spin>
            {/* 拖拽调整宽度手柄 */}
            <div
              className={styles.resizeHandle}
              onMouseDown={(e) => {
                e.preventDefault();
                const startX = e.clientX;
                const startWidth = moduleLibraryWidth;
                const handleMouseMove = (moveEvent: MouseEvent) => {
                  const newWidth = Math.max(150, Math.min(400, startWidth + moveEvent.clientX - startX));
                  setModuleLibraryWidth(newWidth);
                };
                const handleMouseUp = () => {
                  document.removeEventListener('mousemove', handleMouseMove);
                  document.removeEventListener('mouseup', handleMouseUp);
                };
                document.addEventListener('mousemove', handleMouseMove);
                document.addEventListener('mouseup', handleMouseUp);
              }}
            />
          </div>
        )}

        {/* 画布 */}
        <div 
          className={styles.canvas}
          onDrop={handleDrop}
          onDragOver={handleDragOver}
        >
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={handleNodesChange}
            onEdgesChange={handleEdgesChange}
            onConnect={handleConnect}
            onNodeClick={handleNodeClick}
            nodeTypes={nodeTypes}
            snapToGrid
            snapGrid={[15, 15]}
            defaultViewport={{ x: 0, y: 0, zoom: 1 }}
            onMoveEnd={(_, vp) => setViewport(vp)}
            connectionMode={ConnectionMode.Loose}
            fitView
          >
            <Controls />
            <MiniMap />
            <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
            <Panel position="bottom-center">
              <Text type="secondary" style={{ fontSize: 12 }}>
                拖拽节点 | 连接端口 | 双击编辑
              </Text>
            </Panel>
          </ReactFlow>
        </div>

        {/* 右侧属性面板 - 只有选中节点时才显示 */}
        {propertiesPanelOpen && selectedNode && selectedNode.data && (
        <div className={styles.propertiesPanel} style={{ width: propertiesPanelWidth }}>
          {/* 拖拽调整宽度手柄 */}
          <div
            className={styles.resizeHandleLeft}
            onMouseDown={(e) => {
              e.preventDefault();
              const startX = e.clientX;
              const startWidth = propertiesPanelWidth;
              const handleMouseMove = (moveEvent: MouseEvent) => {
                const newWidth = Math.max(180, Math.min(500, startWidth - (moveEvent.clientX - startX)));
                setPropertiesPanelWidth(newWidth);
              };
              const handleMouseUp = () => {
                document.removeEventListener('mousemove', handleMouseMove);
                document.removeEventListener('mouseup', handleMouseUp);
              };
              document.addEventListener('mousemove', handleMouseMove);
              document.addEventListener('mouseup', handleMouseUp);
            }}
          />
          <div className={styles.propertiesPanelHeader}>
            <Text strong>节点属性</Text>
            <Button 
              type="text" 
              size="small" 
              onClick={() => setPropertiesPanelOpen(false)}
            >
              ✕
            </Button>
          </div>
          <div className={styles.propertiesPanelContent}>
            <div>
                <Form layout="vertical" size="small">
                  <Form.Item label="名称">
                    <Input
                      value={selectedNode.data?.instance_name || selectedNode.data?.label || selectedNode.data?.text || ''}
                      onChange={(e) => {
                        const newValue = e.target.value;
                        setSelectedNode((prev: any) => ({
                          ...prev,
                          data: { ...prev.data, instance_name: newValue, label: newValue, text: newValue }
                        }));
                        setNodes((nds: any[]) =>
                          nds.map((n) =>
                            n.id === selectedNode.id
                              ? {
                                  ...n,
                                  data: {
                                    ...n.data,
                                    instance_name: newValue,
                                    label: newValue,
                                    text: newValue,
                                  },
                                }
                              : n
                          )
                        );
                        markChanges();
                      }}
                    />
                  </Form.Item>
                  
                  {/* 背景墙颜色选择 */}
                  {selectedNode.type === 'group' && (
                    <Form.Item label="背景颜色">
                      <Select
                        value={selectedNode.data?.color || '#6495ED'}
                        onChange={(value) => {
                          setSelectedNode((prev: any) => ({
                            ...prev,
                            data: { ...prev.data, color: value }
                          }));
                          setNodes((nds: any[]) =>
                            nds.map((n) =>
                              n.id === selectedNode.id
                                ? { ...n, data: { ...n.data, color: value } }
                                : n
                            )
                          );
                          markChanges();
                        }}
                        options={[
                          { label: '蓝色', value: '#6495ED' },
                          { label: '绿色', value: '#52c41a' },
                          { label: '橙色', value: '#fa8c16' },
                          { label: '红色', value: '#f5222d' },
                          { label: '紫色', value: '#722ed1' },
                          { label: '青色', value: '#13c2c2' },
                          { label: '灰色', value: '#8c8c8c' },
                        ]}
                      />
                    </Form.Item>
                  )}
                  
                  {/* 文字说明颜色选择 */}
                  {selectedNode.type === 'annotation' && (
                    <Form.Item label="文字颜色">
                      <Select
                        value={selectedNode.data?.color || '#666'}
                        onChange={(value) => {
                          setSelectedNode((prev: any) => ({
                            ...prev,
                            data: { ...prev.data, color: value }
                          }));
                          setNodes((nds: any[]) =>
                            nds.map((n) =>
                              n.id === selectedNode.id
                                ? { ...n, data: { ...n.data, color: value } }
                                : n
                            )
                          );
                          markChanges();
                        }}
                        options={[
                          { label: '灰色', value: '#666' },
                          { label: '黑色', value: '#000' },
                          { label: '蓝色', value: '#1890ff' },
                          { label: '绿色', value: '#52c41a' },
                          { label: '红色', value: '#f5222d' },
                        ]}
                      />
                    </Form.Item>
                  )}
                  
                  {selectedNode.type === 'module' && (
                    <>
                      <Form.Item label="Module Source">
                        <Input value={selectedNode.data?.module_source} disabled size="small" />
                      </Form.Item>
                      {selectedNode.data?.module_version && (
                        <Form.Item label="版本">
                          <Input value={selectedNode.data?.module_version} disabled size="small" />
                        </Form.Item>
                      )}
                    </>
                  )}
                </Form>
                
                <Divider style={{ margin: '8px 0' }}>配置参数</Divider>
                
                {/* Demo 选择器和 AI 助手 */}
                {selectedNode.type === 'module' && selectedNode.data?.module_id && (
                  <div style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                    <AIConfigGenerator
                      moduleId={selectedNode.data.module_id}
                      manifestId={manifestId}
                      onGenerate={(config) => {
                        setNodes((nds: any[]) =>
                          nds.map((n) =>
                            n.id === selectedNode.id
                              ? { ...n, data: { ...n.data, config: { ...n.data?.config, ...config } } }
                              : n
                          )
                        );
                        setSelectedNode((prev: any) => ({
                          ...prev,
                          data: { ...prev.data, config: { ...prev.data?.config, ...config } }
                        }));
                        markChanges();
                      }}
                    />
                    <DemoSelector
                      moduleId={selectedNode.data.module_id}
                      onSelectDemo={(demoData, demoName) => {
                        // 检查是否有表单数据
                        const hasData = selectedNode.data?.config && Object.keys(selectedNode.data.config).length > 0;
                        
                        if (hasData) {
                          // 显示确认对话框
                          setPendingDemoData(demoData);
                          setPendingDemoName(demoName);
                          setShowDemoConfirmDialog(true);
                        } else {
                          // 直接应用 Demo 数据
                          setNodes((nds: any[]) =>
                            nds.map((n) =>
                              n.id === selectedNode.id
                                ? { ...n, data: { ...n.data, config: demoData } }
                                : n
                            )
                          );
                          setSelectedNode((prev: any) => ({
                            ...prev,
                            data: { ...prev.data, config: demoData }
                          }));
                          markChanges();
                          toast.success(`已应用 Demo "${demoName}" 的配置`);
                        }
                      }}
                      hasFormData={selectedNode.data?.config && Object.keys(selectedNode.data.config).length > 0}
                    />
                  </div>
                )}
                
                <Spin spinning={schemaLoading}>
                  {nodeSchema?.openapi_schema ? (
                    <ModuleFormRenderer
                      schema={nodeSchema.openapi_schema}
                      initialValues={selectedNode.data?.config || {}}
                      onChange={(values) => {
                        setNodes((nds: any[]) =>
                          nds.map((n) =>
                            n.id === selectedNode.id
                              ? { ...n, data: { ...n.data, config: values } }
                              : n
                          )
                        );
                        markChanges();
                      }}
                      manifest={{
                        currentNodeId: selectedNode.id,
                        // 不限制 connectedNodeIds，允许引用任意节点
                        // 选择引用后会自动创建连线（通过 onAddEdge 回调）
                        connectedNodeIds: undefined,
                        nodes: nodes
                          .filter((n: any) => n.type === 'module' && n.id !== selectedNode.id)
                          .map((n: any) => ({
                            id: n.id,
                            instance_name: n.data?.instance_name || n.data?.label || '',
                            module_id: n.data?.module_id,
                            module_source: n.data?.module_source,
                            outputs: n.data?.outputs || [],
                          })),
                        onAddEdge: (sourceNodeId, targetNodeId, sourceOutput, targetInput) => {
                          console.log('[ManifestEditor] onAddEdge called:', { sourceNodeId, targetNodeId, sourceOutput, targetInput });
                          
                          const sourceInstanceName = nodes.find((n: any) => n.id === sourceNodeId)?.data?.instance_name || '';
                          const newBinding = {
                            sourceOutput,
                            targetInput,
                            expression: `module.${sourceInstanceName}.${sourceOutput}`,
                          };
                          
                          // 使用 setEdges 的函数形式来获取最新的 edges 状态
                          setEdges((currentEdges: any[]) => {
                            // 检查两个节点之间是否已有连线
                            const existingEdge = currentEdges.find((e: any) => 
                              e.source === sourceNodeId && e.target === targetNodeId
                            );
                            
                            if (existingEdge) {
                              // 已有连线，添加新的参数映射到 bindings 数组
                              console.log('[ManifestEditor] Updating existing edge:', existingEdge.id);
                              return currentEdges.map((e) =>
                                e.id === existingEdge.id
                                  ? {
                                      ...e,
                                      data: {
                                        ...e.data,
                                        bindings: [...(e.data?.bindings || []), newBinding],
                                      },
                                    }
                                  : e
                              );
                            } else {
                              // 创建新的 variable_binding 类型的边
                              // 左进右出：source 从右侧出，target 从左侧进
                              const newEdge = {
                                id: `edge-ref-${Date.now()}`,
                                source: sourceNodeId,
                                target: targetNodeId,
                                sourceHandle: 'right',  // 从右侧出
                                targetHandle: 'left',   // 从左侧进
                                type: 'smoothstep',
                                animated: true,
                                style: { stroke: '#52c41a', strokeWidth: 2 },
                                markerEnd: {
                                  type: MarkerType.ArrowClosed,
                                  color: '#52c41a',
                                  width: 15,
                                  height: 15,
                                },
                                data: { 
                                  type: 'variable_binding',
                                  bindings: [newBinding],  // 使用数组存储多个参数映射
                                },
                              };
                              console.log('[ManifestEditor] Creating new edge:', newEdge);
                              return [...currentEdges, newEdge];
                            }
                          });
                          markChanges();
                        },
                      }}
                    />
                  ) : selectedNode.data?.module_id ? (
                    <Text type="secondary" style={{ fontSize: 11 }}>暂无 Schema</Text>
                  ) : (
                    <Text type="secondary" style={{ fontSize: 11 }}>请先关联 Module</Text>
                  )}
                </Spin>
                
                <Divider style={{ margin: '8px 0' }} />
                <Space size="small">
                  <Button
                    size="small"
                    onClick={() => {
                      const newNode = {
                        id: `node-${Date.now()}`,
                        type: 'module',
                        position: { x: selectedNode.position.x + 50, y: selectedNode.position.y + 50 },
                        data: {
                          ...selectedNode.data,
                          instance_name: `${selectedNode.data.instance_name}_copy`,
                          label: `${selectedNode.data.label}_copy`,
                        },
                      };
                      setNodes((nds: any[]) => [...nds, newNode]);
                      markChanges();
                      toast.success('节点已复制');
                    }}
                  >
                    复制
                  </Button>
                  <Button
                    size="small"
                    danger
                    onClick={() => {
                      setNodes((nds: any[]) => nds.filter((n) => n.id !== selectedNode.id));
                      setEdges((eds: any[]) =>
                        eds.filter((e) => e.source !== selectedNode.id && e.target !== selectedNode.id)
                      );
                      setSelectedNode(null);
                      setNodeSchema(null);
                      markChanges();
                    }}
                  >
                    删除
                  </Button>
                </Space>
            </div>
          </div>
        </div>
        )}
      </div>

      {/* 版本历史抽屉 */}
      <Drawer
        title="版本历史"
        placement="right"
        open={versionsDrawerOpen}
        onClose={() => setVersionsDrawerOpen(false)}
        width={400}
      >
        <Spin spinning={versionsLoading}>
          <Timeline
            items={versions.map((version) => ({
              color: version.is_draft ? 'gray' : 'green',
              children: (
                <div key={version.id} style={{ marginBottom: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <Text strong>{version.version}</Text>
                    {version.is_draft ? (
                      <Tag color="default">草稿</Tag>
                    ) : (
                      <Tag color="green">已发布</Tag>
                    )}
                  </div>
                  <Text type="secondary" style={{ fontSize: 12 }}>
                    {new Date(version.created_at).toLocaleString()}
                  </Text>
                  <div style={{ marginTop: 8 }}>
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      {version.nodes?.length || 0} 个节点, {version.edges?.length || 0} 条连接
                    </Text>
                  </div>
                  <div style={{ marginTop: 8 }}>
                    <Space size="small">
                      <Button
                        size="small"
                        icon={<EyeOutlined />}
                        onClick={() => {
                          setSelectedVersion(version);
                          setVersionDetailModalOpen(true);
                        }}
                      >
                        查看
                      </Button>
                      {!version.is_draft && (
                        <Button
                          size="small"
                          icon={<RocketOutlined />}
                          onClick={() => {
                            setVersionsDrawerOpen(false);
                            setDeployModalOpen(true);
                            deployForm.setFieldsValue({ version_id: version.id });
                          }}
                        >
                          部署
                        </Button>
                      )}
                    </Space>
                  </div>
                </div>
              ),
            }))}
          />
          {versions.length === 0 && !versionsLoading && (
            <Text type="secondary">暂无版本历史</Text>
          )}
        </Spin>
      </Drawer>

      {/* 部署对话框 */}
      <Modal
        title="部署到 Workspace"
        open={deployModalOpen}
        onCancel={() => {
          setDeployModalOpen(false);
          deployForm.resetFields();
        }}
        footer={null}
      >
        <Form form={deployForm} layout="vertical" onFinish={async (values) => {
          if (!manifestId) return;
          setDeploying(true);
          try {
            await createManifestDeployment(orgId, manifestId, {
              version_id: values.version_id,
              workspace_id: values.workspace_id,
              auto_apply: values.auto_apply || false,
            });
            toast.success('部署任务已创建');
            setDeployModalOpen(false);
            deployForm.resetFields();
          } catch (error: any) {
            toast.error('部署失败: ' + (error.message || '未知错误'));
          } finally {
            setDeploying(false);
          }
        }}>
          <Form.Item name="version_id" hidden>
            <Input />
          </Form.Item>
          <Form.Item
            name="workspace_id"
            label="目标 Workspace"
            rules={[{ required: true, message: '请选择 Workspace' }]}
          >
            <Select
              placeholder="选择要部署到的 Workspace"
              loading={workspacesLoading}
              showSearch
              optionFilterProp="label"
              options={(Array.isArray(workspaces) ? workspaces : []).map(ws => ({
                label: ws.name,
                value: ws.id,
              }))}
            />
          </Form.Item>
          <Form.Item
            name="auto_apply"
            label="自动 Apply"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0, textAlign: 'right' }}>
            <Space>
              <Button onClick={() => setDeployModalOpen(false)}>取消</Button>
              <Button type="primary" htmlType="submit" loading={deploying}>
                部署
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      {/* HCL 预览对话框 */}
      <Modal
        title="HCL 预览"
        open={previewModalOpen}
        onCancel={() => setPreviewModalOpen(false)}
        width={800}
        footer={
          <Space>
            <Button onClick={() => setPreviewModalOpen(false)}>关闭</Button>
            <Button icon={<CopyOutlined />} onClick={handleCopyHCL}>
              复制
            </Button>
            <Button type="primary" icon={<ExportOutlined />} onClick={() => {
              handleExportHCL();
              setPreviewModalOpen(false);
            }}>
              下载
            </Button>
          </Space>
        }
      >
        <Spin spinning={previewLoading}>
          <pre style={{
            background: '#1e1e1e',
            color: '#d4d4d4',
            padding: 16,
            borderRadius: 8,
            maxHeight: 500,
            overflow: 'auto',
            fontSize: 13,
            fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
            lineHeight: 1.5,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}>
            {previewHCL || '// 加载中...'}
          </pre>
        </Spin>
      </Modal>

      {/* 版本详情对话框 - 只读画布模式 */}
      <Modal
        title={
          <Space>
            <Text strong>版本详情: {selectedVersion?.version || ''}</Text>
            <Tag color={selectedVersion?.is_draft ? 'default' : 'green'}>
              {selectedVersion?.is_draft ? '草稿' : '已发布'}
            </Tag>
            <Tag color="orange">只读模式</Tag>
          </Space>
        }
        open={versionDetailModalOpen}
        onCancel={() => {
          setVersionDetailModalOpen(false);
          setSelectedVersion(null);
        }}
        width="90%"
        style={{ top: 20 }}
        styles={{ body: { height: 'calc(100vh - 200px)', padding: 0 } }}
        footer={
          <Space>
            <Text type="secondary" style={{ marginRight: 16 }}>
              创建时间: {selectedVersion ? new Date(selectedVersion.created_at).toLocaleString() : '-'}
            </Text>
            <Button onClick={() => setVersionDetailModalOpen(false)}>关闭</Button>
            {selectedVersion && !selectedVersion.is_draft && (
              <Button
                type="primary"
                icon={<RocketOutlined />}
                onClick={() => {
                  setVersionDetailModalOpen(false);
                  setVersionsDrawerOpen(false);
                  setDeployModalOpen(true);
                  deployForm.setFieldsValue({ version_id: selectedVersion.id });
                }}
              >
                部署此版本
              </Button>
            )}
          </Space>
        }
      >
        {selectedVersion && (
          <div style={{ display: 'flex', height: '100%' }}>
            {/* 只读画布 */}
            <div style={{ flex: 1, height: '100%' }}>
              <ReactFlow
                nodes={convertToFlowNodes(selectedVersion.nodes || [])}
                edges={convertToFlowEdges(selectedVersion.edges || [])}
                nodeTypes={nodeTypes}
                fitView
                nodesDraggable={false}
                nodesConnectable={false}
                elementsSelectable={false}
                panOnDrag={true}
                zoomOnScroll={true}
                defaultViewport={selectedVersion.canvas_data?.viewport ? {
                  x: selectedVersion.canvas_data.viewport.x || 0,
                  y: selectedVersion.canvas_data.viewport.y || 0,
                  zoom: selectedVersion.canvas_data.zoom || 1,
                } : { x: 0, y: 0, zoom: 1 }}
              >
                <Controls showInteractive={false} />
                <MiniMap />
                <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
                <Panel position="top-left">
                  <Tag color="blue">{selectedVersion.nodes?.length || 0} 个节点</Tag>
                  <Tag color="green">{selectedVersion.edges?.length || 0} 条连接</Tag>
                </Panel>
              </ReactFlow>
            </div>
            
            {/* 右侧节点详情面板 */}
            <div style={{ 
              width: 350, 
              borderLeft: '1px solid #f0f0f0', 
              padding: 16, 
              overflow: 'auto',
              background: '#fafafa',
            }}>
              <Text strong style={{ fontSize: 14, marginBottom: 12, display: 'block' }}>
                节点配置详情
              </Text>
              {(selectedVersion.nodes || []).filter((n: any) => n.type === 'module').map((node: any, index: number) => (
                <Card 
                  key={node.id || index} 
                  size="small" 
                  style={{ marginBottom: 8 }}
                  title={
                    <Text strong style={{ fontSize: 12 }}>
                      {node.instance_name || node.resource_name || '未命名'}
                    </Text>
                  }
                >
                  <div style={{ marginBottom: 4 }}>
                    <Text type="secondary" style={{ fontSize: 11 }}>Source: </Text>
                    <Text code style={{ fontSize: 10 }}>{node.module_source || '-'}</Text>
                  </div>
                  {node.module_version && (
                    <div style={{ marginBottom: 4 }}>
                      <Text type="secondary" style={{ fontSize: 11 }}>Version: </Text>
                      <Text code style={{ fontSize: 10 }}>{node.module_version}</Text>
                    </div>
                  )}
                  {node.config && Object.keys(node.config).length > 0 && (
                    <div>
                      <Text type="secondary" style={{ fontSize: 11 }}>配置参数:</Text>
                      <pre style={{
                        background: '#fff',
                        padding: 8,
                        borderRadius: 4,
                        fontSize: 10,
                        maxHeight: 200,
                        overflow: 'auto',
                        marginTop: 4,
                        border: '1px solid #e8e8e8',
                      }}>
                        {JSON.stringify(node.config, null, 2)}
                      </pre>
                    </div>
                  )}
                </Card>
              ))}
              {(selectedVersion.nodes || []).filter((n: any) => n.type === 'module').length === 0 && (
                <Text type="secondary">暂无 Module 节点</Text>
              )}
            </div>
          </div>
        )}
      </Modal>

      {/* Demo 选择确认对话框 */}
      <ConfirmDialog
        isOpen={showDemoConfirmDialog}
        title="确认使用 Demo 配置"
        message="选择 Demo 将覆盖当前已填写的配置数据，是否继续？"
        confirmText="确认使用"
        cancelText="取消"
        onConfirm={() => {
          if (pendingDemoData && selectedNode) {
            setNodes((nds: any[]) =>
              nds.map((n) =>
                n.id === selectedNode.id
                  ? { ...n, data: { ...n.data, config: pendingDemoData } }
                  : n
              )
            );
            setSelectedNode((prev: any) => ({
              ...prev,
              data: { ...prev.data, config: pendingDemoData }
            }));
            markChanges();
            toast.success(`已应用 Demo "${pendingDemoName}" 的配置`);
          }
          setShowDemoConfirmDialog(false);
          setPendingDemoData(null);
          setPendingDemoName('');
        }}
        onCancel={() => {
          setShowDemoConfirmDialog(false);
          setPendingDemoData(null);
          setPendingDemoName('');
        }}
        type="warning"
      />

      {/* 发布版本对话框 */}
      <Modal
        title="发布版本"
        open={publishModalOpen}
        onCancel={() => {
          setPublishModalOpen(false);
          publishForm.resetFields();
        }}
        footer={null}
      >
        <Form form={publishForm} layout="vertical" onFinish={handlePublish}>
          <Form.Item
            name="version"
            label="版本号"
            rules={[
              { required: true, message: '请输入版本号' },
              {
                pattern: /^v?\d+\.\d+\.\d+$/,
                message: '版本号格式应为 v1.0.0 或 1.0.0',
              },
            ]}
          >
            <Input placeholder="例如: v1.0.0" />
          </Form.Item>
          <Form.Item style={{ marginBottom: 0, textAlign: 'right' }}>
            <Space>
              <Button onClick={() => setPublishModalOpen(false)}>取消</Button>
              <Button type="primary" htmlType="submit" loading={publishing}>
                发布
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

// 包装组件，提供 ReactFlowProvider
const ManifestEditor: React.FC = () => {
  return (
    <ReactFlowProvider>
      <ManifestEditorInner />
    </ReactFlowProvider>
  );
};

export default ManifestEditor;
