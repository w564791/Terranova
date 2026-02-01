import React, { memo, useState, useRef, useEffect } from 'react';
import type { NodeProps } from '@xyflow/react';
import { NodeResizer } from '@xyflow/react';
import styles from './AnnotationNode.module.css';

interface AnnotationNodeData {
  text?: string;
  fontSize?: number;
  color?: string;
  onTextChange?: (text: string) => void;
  [key: string]: unknown;
}

const AnnotationNode: React.FC<NodeProps> = ({ data, selected }) => {
  const nodeData = data as AnnotationNodeData;
  const [isEditing, setIsEditing] = useState(false);
  const [text, setText] = useState(nodeData.text || '双击编辑文字');
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (isEditing && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [isEditing]);

  const handleDoubleClick = () => {
    setIsEditing(true);
  };

  const handleBlur = () => {
    setIsEditing(false);
    // 通知父组件文字已更改
    if (nodeData.onTextChange) {
      nodeData.onTextChange(text);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      setIsEditing(false);
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      setIsEditing(false);
      if (nodeData.onTextChange) {
        nodeData.onTextChange(text);
      }
    }
  };

  return (
    <>
      <NodeResizer
        color="#999"
        isVisible={selected}
        minWidth={50}
        minHeight={20}
      />
      <div
        className={`${styles.annotationNode} ${selected ? styles.selected : ''}`}
        style={{
          fontSize: nodeData.fontSize || 12,
          color: nodeData.color || '#666',
        }}
        onDoubleClick={handleDoubleClick}
      >
        {isEditing ? (
          <textarea
            ref={inputRef}
            className={styles.editInput}
            value={text}
            onChange={(e) => setText(e.target.value)}
            onBlur={handleBlur}
            onKeyDown={handleKeyDown}
            style={{
              fontSize: nodeData.fontSize || 12,
              color: nodeData.color || '#666',
            }}
          />
        ) : (
          text || '双击编辑文字'
        )}
      </div>
    </>
  );
};

export default memo(AnnotationNode);
