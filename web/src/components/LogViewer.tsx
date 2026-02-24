import { useState, useEffect, useRef, useCallback } from 'react';
import {
  RefreshCw,
  ArrowDown,
  ArrowDownToLine,
  Filter,
  Loader2,
} from 'lucide-react';
import { getSandboxLogs, parseLogLevel, type LogLevel } from '../api/client';

interface LogViewerProps {
  sandboxId: string;
}

const levelColors: Record<LogLevel, string> = {
  INFO: 'text-gray-300',
  WARN: 'text-amber-400',
  ERROR: 'text-red-400',
  DEBUG: 'text-gray-500',
};

const levelBadgeColors: Record<LogLevel, string> = {
  INFO: 'bg-blue-500/20 text-blue-400',
  WARN: 'bg-amber-500/20 text-amber-400',
  ERROR: 'bg-red-500/20 text-red-400',
  DEBUG: 'bg-gray-500/20 text-gray-400',
};

export default function LogViewer({ sandboxId }: LogViewerProps) {
  const [lines, setLines] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [lineCount, setLineCount] = useState(100);
  const [follow, setFollow] = useState(true);
  const [levelFilter, setLevelFilter] = useState<Set<LogLevel>>(
    new Set(['INFO', 'WARN', 'ERROR', 'DEBUG']),
  );
  const [showFilter, setShowFilter] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchLogs = useCallback(async () => {
    try {
      const data = await getSandboxLogs(sandboxId, lineCount);
      setLines(data);
    } catch {
      // silently fail on refresh
    } finally {
      setLoading(false);
    }
  }, [sandboxId, lineCount]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  // Auto-refresh when follow is on
  useEffect(() => {
    if (follow) {
      intervalRef.current = setInterval(fetchLogs, 2000);
    }
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [follow, fetchLogs]);

  // Auto-scroll
  useEffect(() => {
    if (follow && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [lines, follow]);

  const toggleLevel = (level: LogLevel) => {
    setLevelFilter((prev) => {
      const next = new Set(prev);
      if (next.has(level)) {
        next.delete(level);
      } else {
        next.add(level);
      }
      return next;
    });
  };

  const filteredLines = lines.filter((line) => levelFilter.has(parseLogLevel(line)));

  return (
    <div className="flex flex-col h-96 bg-navy-950 rounded-lg border border-navy-600 overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2 bg-navy-900 border-b border-navy-600">
        <div className="flex items-center gap-2 text-sm text-gray-400">
          <ArrowDown className="w-4 h-4" />
          <span>Console</span>
          <span className="text-xs text-gray-600">
            {filteredLines.length} line{filteredLines.length !== 1 ? 's' : ''}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {/* Line count selector */}
          <select
            value={lineCount}
            onChange={(e) => setLineCount(Number(e.target.value))}
            className="text-xs bg-navy-800 border border-navy-600 rounded px-2 py-1 text-gray-300"
          >
            <option value={50}>50 lines</option>
            <option value={100}>100 lines</option>
            <option value={500}>500 lines</option>
            <option value={1000}>1000 lines</option>
          </select>

          {/* Filter toggle */}
          <button
            onClick={() => setShowFilter(!showFilter)}
            className={`p-1 rounded transition-colors ${
              showFilter ? 'text-primary-400 bg-primary-500/10' : 'text-gray-500 hover:text-gray-300'
            }`}
            title="Filter levels"
          >
            <Filter className="w-3.5 h-3.5" />
          </button>

          {/* Follow toggle */}
          <button
            onClick={() => setFollow(!follow)}
            className={`p-1 rounded transition-colors ${
              follow ? 'text-emerald-400 bg-emerald-500/10' : 'text-gray-500 hover:text-gray-300'
            }`}
            title={follow ? 'Following (click to pause)' : 'Paused (click to follow)'}
          >
            <ArrowDownToLine className="w-3.5 h-3.5" />
          </button>

          {/* Manual refresh */}
          <button
            onClick={fetchLogs}
            className="text-gray-500 hover:text-gray-300 transition-colors"
            title="Refresh"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Level filter bar */}
      {showFilter && (
        <div className="flex items-center gap-2 px-4 py-2 bg-navy-900/50 border-b border-navy-600">
          {(['INFO', 'WARN', 'ERROR', 'DEBUG'] as LogLevel[]).map((level) => (
            <button
              key={level}
              onClick={() => toggleLevel(level)}
              className={`text-xs px-2 py-0.5 rounded-full transition-colors ${
                levelFilter.has(level)
                  ? levelBadgeColors[level]
                  : 'bg-navy-700 text-gray-600'
              }`}
            >
              {level}
            </button>
          ))}
        </div>
      )}

      {/* Log output */}
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto p-3 font-mono text-xs"
      >
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="w-5 h-5 animate-spin text-gray-500" />
          </div>
        ) : filteredLines.length === 0 ? (
          <div className="text-gray-600 text-center py-8">
            No console output available
          </div>
        ) : (
          filteredLines.map((line, i) => {
            const level = parseLogLevel(line);
            return (
              <div key={i} className="flex gap-2 leading-relaxed hover:bg-navy-800/50">
                <span className="text-gray-600 select-none w-8 text-right flex-shrink-0">
                  {i + 1}
                </span>
                <pre className={`whitespace-pre-wrap break-all ${levelColors[level]}`}>
                  {line}
                </pre>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
