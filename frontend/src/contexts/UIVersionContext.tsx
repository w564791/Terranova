import React, { createContext, useContext } from 'react';
import type { UIVersion } from '../hooks/useUIVersion';

interface UIVersionContextValue {
  version: UIVersion;
  isV3: boolean;
}

const UIVersionContext = createContext<UIVersionContextValue>({
  version: 'v2',
  isV3: false,
});

export const UIVersionProvider = UIVersionContext.Provider;

export function useUIVersionContext(): UIVersionContextValue {
  return useContext(UIVersionContext);
}
