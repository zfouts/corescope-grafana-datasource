import { useEffect, useState } from 'react';
import { MetricFindOption } from '../types';

/** Loads an options list once, the first time `enabled` is true. A failed load yields an empty list; custom values still work. */
export function useOptions(load: () => Promise<MetricFindOption[]>, enabled = true) {
  const [options, setOptions] = useState<MetricFindOption[]>([]);
  const [loading, setLoading] = useState(enabled);
  useEffect(() => {
    if (!enabled) {
      return;
    }
    let live = true;
    load()
      .then((o) => live && setOptions(o))
      .catch(() => live && setOptions([]))
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled]);
  return { options, loading };
}
