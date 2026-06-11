// Manifest AI 工具的内联样式(贴合 VS Code 暗色主题)。从 ManifestAiTools.tsx 抽出以控制单文件体积。
import type React from 'react'

// 生成面板展开时的固定宽度(与父组件挤占的 marginRight 一致)
export const AI_PANEL_WIDTH = 360

// ===== 内联样式(贴合 VS Code 暗色主题)=====
// 右侧停靠聊天面板。宽度与父组件挤占的 marginRight(AI_PANEL_WIDTH)一致,
// 面板正好填进让出的右侧空槽,不再悬浮遮挡编辑器。
export const chatPanelStyle: React.CSSProperties = {
  position: 'absolute',
  top: 65, // 让出 titleBar(30) + toolbar(35)
  right: 0,
  bottom: 22, // 让出 statusBar
  width: AI_PANEL_WIDTH,
  background: '#1e1e1e',
  borderLeft: '1px solid #2d2d2d',
  display: 'flex',
  flexDirection: 'column',
  zIndex: 60,
  userSelect: 'text', // 覆盖 .toolbar 继承的 user-select:none,允许复制面板内容
}
export const chatHeaderStyle: React.CSSProperties = {
  position: 'relative',
  display: 'flex',
  alignItems: 'center',
  gap: 6,
  padding: '8px 12px',
  borderBottom: '1px solid #2d2d2d',
  fontSize: 13,
}
export const chatHeaderUnderline: React.CSSProperties = {
  position: 'absolute',
  left: 12,
  bottom: -1,
  width: 28,
  height: 2,
  background: '#e8843c',
}
export const chatHeaderIcon: React.CSSProperties = {
  cursor: 'pointer',
  fontSize: 15,
  opacity: 0.8,
  padding: '0 4px',
  color: '#cccccc',
}
export const chatBodyStyle: React.CSSProperties = {
  flex: 1,
  overflow: 'auto',
  padding: 12,
  color: '#cccccc',
}
export const chatEmptyStyle: React.CSSProperties = {
  height: '100%',
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 8,
  color: '#cccccc',
}
export const chatProgressStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  fontSize: 13,
  color: '#9cdcfe',
  padding: '6px 0',
}
// 历史会话下拉
export const sessionListStyle: React.CSSProperties = {
  position: 'absolute',
  top: 36,
  right: 8,
  width: 280,
  maxHeight: 320,
  overflow: 'auto',
  background: '#252526',
  border: '1px solid #454545',
  borderRadius: 4,
  zIndex: 70,
  boxShadow: '0 4px 16px rgba(0,0,0,0.4)',
}
export const sessionItemStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '6px 10px',
  fontSize: 12,
  color: '#cccccc',
  cursor: 'pointer',
  borderBottom: '1px solid #2d2d2d',
}
// 历史消息
export const historyMsgStyle: React.CSSProperties = {
  padding: '8px 0',
  borderBottom: '1px solid #2a2a2a',
}
export const historyTextStyle: React.CSSProperties = {
  fontSize: 13,
  color: '#cccccc',
  whiteSpace: 'pre-wrap',
}
export const historyCodeStyle: React.CSSProperties = {
  fontSize: 12,
  background: '#1b1b1b',
  border: '1px solid #2d2d2d',
  borderRadius: 4,
  padding: 8,
  overflow: 'auto',
  maxHeight: 200,
  color: '#d4d4d4',
}
export const historyIssueStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 6,
  padding: '3px 0',
  fontSize: 12,
  color: '#cccccc',
  cursor: 'pointer',
}
export const chatInputWrapStyle: React.CSSProperties = {
  margin: 12,
  border: '1px solid #3c3c3c',
  borderRadius: 6,
  background: '#252526',
}
export const contextChipRowStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 6,
  padding: '8px 8px 0',
}
export const contextChipStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  padding: '2px 8px',
  border: '1px solid #3c3c3c',
  borderRadius: 4,
  background: '#2d2d2d',
  fontSize: 12,
  color: '#cccccc',
  maxWidth: '100%',
}
export const chatInputStyle: React.CSSProperties = {
  width: '100%',
  background: 'transparent',
  color: '#cccccc',
  border: 'none',
  outline: 'none',
  padding: 10,
  fontFamily: 'inherit',
  fontSize: 13,
  resize: 'none',
  boxSizing: 'border-box',
}
export const chatInputFooterStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '4px 10px 8px',
  color: '#cccccc',
}
export const errorStyle: React.CSSProperties = {
  marginTop: 10,
  padding: '6px 10px',
  background: 'rgba(241,76,76,0.12)',
  border: '1px solid rgba(241,76,76,0.4)',
  borderRadius: 4,
  color: '#f14c4c',
  fontSize: 13,
}
export const genWarnStyle: React.CSSProperties = {
  marginTop: 10,
  padding: '6px 10px',
  background: 'rgba(204,167,0,0.12)',
  border: '1px solid rgba(204,167,0,0.4)',
  borderRadius: 4,
  color: '#cca700',
  fontSize: 13,
}
export const pipelineStyle: React.CSSProperties = {
  padding: '8px 12px',
  borderBottom: '1px solid #2d2d2d',
  background: '#1b1b1b',
  fontSize: 12,
}
export const pipelineStepStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  flexWrap: 'wrap',
  padding: '3px 0',
  color: '#cccccc',
}
export const pipelineSkillStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 4,
  flexBasis: '100%',
  paddingLeft: 22,
}
export const pipelineSkillTagStyle: React.CSSProperties = {
  padding: '0 6px',
  borderRadius: 3,
  background: '#2d2d2d',
  border: '1px solid #3c3c3c',
  fontSize: 11,
  color: '#9cdcfe',
}
export const issuePanelStyle: React.CSSProperties = {
  position: 'absolute',
  left: 48, // 让出 activityBar
  right: 0,
  bottom: 22, // 让出底部 statusBar
  height: 200,
  background: '#1e1e1e',
  borderTop: '1px solid #454545',
  display: 'flex',
  flexDirection: 'column',
  zIndex: 50,
}
export const issueHeaderStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '6px 12px',
  borderBottom: '1px solid #2d2d2d',
  background: '#252526',
  color: '#cccccc',
  fontSize: 13,
  textTransform: 'uppercase',
}
export const issueBodyStyle: React.CSSProperties = {
  flex: 1,
  overflow: 'auto',
}
export const issueRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '4px 12px',
  fontSize: 13,
  color: '#cccccc',
  borderBottom: '1px solid #2a2a2a',
}
export const fixBtnStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  padding: '1px 8px',
  fontSize: 12,
  background: '#0e639c',
  color: '#fff',
  border: 'none',
  borderRadius: 3,
  cursor: 'pointer',
}
export const fixBtnDisabledStyle: React.CSSProperties = {
  ...fixBtnStyle,
  background: '#3a3a3a',
  color: '#888',
  cursor: 'default',
}
export const fixAppliedBannerStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 6,
  padding: '6px 12px',
  fontSize: 12,
  background: 'rgba(204,167,0,0.12)',
  borderBottom: '1px solid rgba(204,167,0,0.3)',
  color: '#cca700',
}
