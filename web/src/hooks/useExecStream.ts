import { useState, useCallback, useRef } from 'react';
import { type ExecResult, type StreamChunk, execCommand } from '../api/client';

export interface ExecHistoryEntry {
  id: number;
  command: string;
  result: ExecResult | null;
  error: string | null;
  running: boolean;
  timestamp: Date;
  streamOutput: StreamChunk[];
}

interface UseExecStreamReturn {
  history: ExecHistoryEntry[];
  running: boolean;
  execute: (sandboxId: string, command: string) => Promise<void>;
  cancelExec: () => void;
  clearHistory: () => void;
}

let nextId = 1;

export function useExecStream(): UseExecStreamReturn {
  const [history, setHistory] = useState<ExecHistoryEntry[]>([]);
  const [running, setRunning] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const mountedRef = useRef(true);

  const execute = useCallback(
    async (sandboxId: string, command: string): Promise<void> => {
      const entryId = nextId++;
      const entry: ExecHistoryEntry = {
        id: entryId,
        command,
        result: null,
        error: null,
        running: true,
        timestamp: new Date(),
        streamOutput: [],
      };

      setHistory((prev) => [...prev, entry]);
      setRunning(true);

      const controller = new AbortController();
      abortRef.current = controller;

      try {
        const result = await execCommand(sandboxId, command);
        if (!mountedRef.current) return;

        setHistory((prev) =>
          prev.map((e) =>
            e.id === entryId
              ? { ...e, running: false, result }
              : e,
          ),
        );
      } catch (err) {
        if (!mountedRef.current) return;
        if (controller.signal.aborted) {
          setHistory((prev) =>
            prev.map((e) =>
              e.id === entryId
                ? { ...e, running: false, error: 'Cancelled' }
                : e,
            ),
          );
        } else {
          const errorMsg = err instanceof Error ? err.message : 'Exec failed';
          setHistory((prev) =>
            prev.map((e) =>
              e.id === entryId
                ? { ...e, running: false, error: errorMsg }
                : e,
            ),
          );
        }
      } finally {
        if (mountedRef.current) {
          setRunning(false);
          abortRef.current = null;
        }
      }
    },
    [],
  );

  const cancelExec = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const clearHistory = useCallback(() => {
    setHistory([]);
  }, []);

  return { history, running, execute, cancelExec, clearHistory };
}
