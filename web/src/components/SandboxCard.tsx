import {
  Box,
  Clock,
  Cpu,
  Trash2,
  Copy,
  ChevronDown,
  ChevronUp,
  TimerReset,
} from 'lucide-react';
import type { Sandbox } from '../api/client';

interface SandboxCardProps {
  sandbox: Sandbox;
  expanded: boolean;
  onToggle: () => void;
  onDestroy: (id: string) => void;
  onExtend?: (id: string) => void;
  children?: React.ReactNode;
  selectable?: boolean;
  selected?: boolean;
  onSelect?: (id: string) => void;
}

function statusBadge(status: string) {
  switch (status) {
    case 'running':
      return 'badge-success';
    case 'stopped':
    case 'expired':
      return 'badge-danger';
    case 'creating':
      return 'badge-warning';
    default:
      return 'badge-neutral';
  }
}

function formatDate(dateStr: string): string {
  try {
    const d = new Date(dateStr);
    return d.toLocaleString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return dateStr;
  }
}

function timeRemaining(expiresAt: string): string {
  try {
    const expires = new Date(expiresAt).getTime();
    const now = Date.now();
    const diff = expires - now;
    if (diff <= 0) return 'expired';
    const minutes = Math.floor(diff / 60000);
    if (minutes < 60) return `${minutes}m remaining`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ${minutes % 60}m remaining`;
    const days = Math.floor(hours / 24);
    return `${days}d ${hours % 24}h remaining`;
  } catch {
    return '';
  }
}

export default function SandboxCard({
  sandbox,
  expanded,
  onToggle,
  onDestroy,
  onExtend,
  children,
  selectable,
  selected,
  onSelect,
}: SandboxCardProps) {
  const copyId = () => {
    navigator.clipboard.writeText(sandbox.id);
  };

  return (
    <div className={`card !p-0 overflow-hidden animate-fade-in transition-all duration-200 ${selected ? 'ring-2 ring-primary-500/50 border-primary-500/30' : ''}`}>
      {/* Header row */}
      <div
        className="flex items-center gap-4 px-5 py-4 cursor-pointer hover:bg-navy-600/30 transition-colors"
        onClick={onToggle}
      >
        {selectable && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              onSelect?.(sandbox.id);
            }}
            className="flex-shrink-0"
          >
            <div
              className={`w-5 h-5 rounded border-2 flex items-center justify-center transition-all duration-150 ${
                selected
                  ? 'bg-primary-500 border-primary-500'
                  : 'border-navy-400 hover:border-gray-300'
              }`}
            >
              {selected && (
                <svg className="w-3 h-3 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                </svg>
              )}
            </div>
          </button>
        )}
        <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-primary-500/10">
          <Box className="w-5 h-5 text-primary-400" />
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-mono font-semibold text-gray-200 truncate">
              {sandbox.id}
            </span>
            <button
              onClick={(e) => {
                e.stopPropagation();
                copyId();
              }}
              className="text-gray-600 hover:text-gray-400 transition-colors"
              title="Copy ID"
            >
              <Copy className="w-3.5 h-3.5" />
            </button>
          </div>
          <div className="flex items-center gap-3 mt-0.5 text-xs text-gray-500">
            <span className="flex items-center gap-1">
              <Cpu className="w-3 h-3" />
              {sandbox.image}
            </span>
            <span>{sandbox.provider}</span>
          </div>
        </div>

        <div className="flex items-center gap-3">
          <span className={statusBadge(sandbox.status)}>{sandbox.status}</span>
          <div className="hidden sm:flex flex-col items-end text-xs text-gray-500">
            <span className="flex items-center gap-1">
              <Clock className="w-3 h-3" />
              {formatDate(sandbox.created_at)}
            </span>
            {sandbox.expires_at && (
              <span className="text-gray-600">
                {timeRemaining(sandbox.expires_at)}
              </span>
            )}
          </div>
          <button
            onClick={(e) => {
              e.stopPropagation();
              onDestroy(sandbox.id);
            }}
            className="p-1.5 rounded-lg text-gray-500 hover:text-red-400 hover:bg-red-500/10 transition-colors"
            title="Destroy sandbox"
          >
            <Trash2 className="w-4 h-4" />
          </button>
          {expanded ? (
            <ChevronUp className="w-5 h-5 text-gray-500" />
          ) : (
            <ChevronDown className="w-5 h-5 text-gray-500" />
          )}
        </div>
      </div>

      {/* Expanded detail panel */}
      {expanded && (
        <div className="border-t border-navy-600">
          {/* Info row */}
          <div className="px-5 py-3 bg-navy-800/50 flex flex-wrap gap-x-6 gap-y-1 text-xs">
            <span>
              <span className="text-gray-500">IP:</span>{' '}
              <span className="font-mono text-gray-300">
                {sandbox.ip || 'N/A'}
              </span>
            </span>
            <span>
              <span className="text-gray-500">TTL:</span>{' '}
              <span className="font-mono text-gray-300">
                {sandbox.ttl || 'N/A'}
              </span>
            </span>
            <span>
              <span className="text-gray-500">Created:</span>{' '}
              <span className="text-gray-300">
                {formatDate(sandbox.created_at)}
              </span>
            </span>
            <span className="flex items-center gap-1">
              <span className="text-gray-500">Expires:</span>{' '}
              <span className="text-gray-300">
                {sandbox.expires_at ? formatDate(sandbox.expires_at) : 'never'}
              </span>
              {onExtend && sandbox.status === 'running' && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onExtend(sandbox.id);
                  }}
                  className="ml-1 inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[10px] font-medium bg-primary-500/10 text-primary-400 hover:bg-primary-500/20 transition-colors"
                  title="Extend TTL by 30 minutes"
                >
                  <TimerReset className="w-3 h-3" />
                  +30m
                </button>
              )}
            </span>
          </div>

          {/* Tab content (terminal, file browser, etc.) */}
          <div className="p-4">{children}</div>
        </div>
      )}
    </div>
  );
}
