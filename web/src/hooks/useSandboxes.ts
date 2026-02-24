import { useState, useEffect, useCallback, useRef } from 'react';
import {
  type Sandbox,
  type SSEEvent,
  listSandboxes,
  subscribeEvents,
} from '../api/client';

interface UseSandboxesReturn {
  sandboxes: Sandbox[];
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  events: SSEEvent[];
}

const POLL_INTERVAL = 5000;
const MAX_EVENTS = 100;

export function useSandboxes(): UseSandboxesReturn {
  const [sandboxes, setSandboxes] = useState<Sandbox[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [events, setEvents] = useState<SSEEvent[]>([]);
  const mountedRef = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const data = await listSandboxes();
      if (mountedRef.current) {
        setSandboxes(data);
        setError(null);
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(err instanceof Error ? err.message : 'Failed to fetch sandboxes');
      }
    } finally {
      if (mountedRef.current) {
        setLoading(false);
      }
    }
  }, []);

  // Initial fetch + polling
  useEffect(() => {
    mountedRef.current = true;
    refresh();

    const interval = setInterval(refresh, POLL_INTERVAL);

    return () => {
      mountedRef.current = false;
      clearInterval(interval);
    };
  }, [refresh]);

  // SSE subscription
  useEffect(() => {
    const unsubscribe = subscribeEvents(
      (event: SSEEvent) => {
        if (!mountedRef.current) return;

        setEvents((prev) => {
          const updated = [event, ...prev];
          return updated.slice(0, MAX_EVENTS);
        });

        // Refresh sandboxes on relevant events
        if (
          event.type.startsWith('sandbox.') ||
          event.type === 'message'
        ) {
          refresh();
        }
      },
      () => {
        // SSE errors are expected when backend restarts; silently reconnect
      },
    );

    return () => {
      unsubscribe();
    };
  }, [refresh]);

  return { sandboxes, loading, error, refresh, events };
}
