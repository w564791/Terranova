import React, { memo } from 'react';
import type { NodeProps } from '@xyflow/react';
import { NodeResizer } from '@xyflow/react';

interface GroupNodeData {
  label?: string;
  color?: string;
  [key: string]: unknown;
}

const GroupNode: React.FC<NodeProps> = ({ data, selected }) => {
  const nodeData = data as GroupNodeData;
  const baseColor = nodeData.color || '#6495ED';

  return (
    <div
      style={{
        width: '100%',
        height: '100%',
        backgroundColor: `${baseColor}60`,
        border: `2px solid ${baseColor}`,
        borderRadius: 8,
        position: 'relative',
      }}
    >
      <NodeResizer
        color={baseColor}
        isVisible={selected}
        minWidth={100}
        minHeight={80}
      />
      {nodeData.label && (
        <div
          style={{
            position: 'absolute',
            top: 4,
            left: 8,
            fontSize: 10,
            fontWeight: 500,
            color: baseColor,
            backgroundColor: 'rgba(255, 255, 255, 0.85)',
            padding: '1px 6px',
            borderRadius: 3,
          }}
        >
          {nodeData.label}
        </div>
      )}
    </div>
  );
};

export default memo(GroupNode);
