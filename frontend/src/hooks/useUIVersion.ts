import { useState, useCallback, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';

export type UIVersion = 'v2' | 'v3';

const STORAGE_KEY = 'ui-version-preference';

function readFromStorage(): UIVersion {
  return (localStorage.getItem(STORAGE_KEY) as UIVersion) || 'v2';
}

export function useUIVersion() {
  const [searchParams, setSearchParams] = useSearchParams();
  const urlVersion = searchParams.get('ui');

  const [version, setVersionState] = useState<UIVersion>(() => {
    if (urlVersion === 'v2' || urlVersion === 'v3') return urlVersion;
    return readFromStorage();
  });

  const setVersion = useCallback((v: UIVersion) => {
    localStorage.setItem(STORAGE_KEY, v);
    setVersionState(v);
    document.documentElement.setAttribute('data-ui-version', v);
    window.dispatchEvent(new CustomEvent('ui-version-change', { detail: v }));
    setSearchParams(prev => {
      const next = new URLSearchParams(prev);
      next.set('ui', v);
      return next;
    }, { replace: true });
  }, [setSearchParams]);

  // document.documentElement attribute for CSS scoping
  useEffect(() => {
    document.documentElement.setAttribute('data-ui-version', version);
    return () => {
      document.documentElement.removeAttribute('data-ui-version');
    };
  }, [version]);

  // URL param changes (from other components using setSearchParams)
  useEffect(() => {
    if (urlVersion && (urlVersion === 'v2' || urlVersion === 'v3') && urlVersion !== version) {
      localStorage.setItem(STORAGE_KEY, urlVersion);
      setVersionState(urlVersion);
    }
  }, [urlVersion, version]);

  // localStorage changes (cross-tab + same-page fallback via manual dispatch)
  useEffect(() => {
    const handleStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY && e.newValue && (e.newValue === 'v2' || e.newValue === 'v3')) {
        setVersionState(e.newValue as UIVersion);
        document.documentElement.setAttribute('data-ui-version', e.newValue);
        setSearchParams(prev => {
          const next = new URLSearchParams(prev);
          next.set('ui', e.newValue!);
          return next;
        }, { replace: true });
      }
    };
    window.addEventListener('storage', handleStorage);
    return () => window.removeEventListener('storage', handleStorage);
  }, [setSearchParams]);

  // Custom event for same-page sync (storage event only fires cross-tab)
  useEffect(() => {
    const handleCustom = (e: Event) => {
      const detail = (e as CustomEvent).detail;
      if (detail && (detail === 'v2' || detail === 'v3') && detail !== version) {
        setVersionState(detail);
        document.documentElement.setAttribute('data-ui-version', detail);
      }
    };
    window.addEventListener('ui-version-change', handleCustom);
    return () => window.removeEventListener('ui-version-change', handleCustom);
  }, [version]);

  return {
    version,
    setVersion,
    isV3: version === 'v3',
  };
}
