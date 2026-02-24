import { useState, useEffect } from 'react';
import {
  Box,
  FileCode2,
  Server,
  Activity,
  Cpu,
  HardDrive,
  Clock,
  Zap,
  RefreshCw,
  Plus,
  Egg,
} from 'lucide-react';
import {
  type HealthResponse,
  type MetricsResponse,
  type SSEEvent,
  getHealth,
  getMetrics,
  listTemplates,
  listProviders,
} from '../api/client';
import { useSandboxes } from '../hooks/useSandboxes';
import { StatCardSkeleton } from '../components/Skeleton';

interface StatCardProps {
  label: string;
  value: string | number;
  icon: React.ReactNode;
  color: string;
  subtitle?: string;
  large?: boolean;
}

function StatCard({ label, value, icon, color, subtitle, large }: StatCardProps) {
  return (
    <div className={`card animate-fade-in hover:border-navy-500 transition-all duration-200 ${large ? 'sm:col-span-2' : ''}`}>
      <div className="flex items-start justify-between">
        <div>
          <p className="text-sm font-medium text-gray-400">{label}</p>
          <p className={`mt-1 font-display font-bold text-gray-100 ${large ? 'text-4xl' : 'text-3xl'}`}>
            {value}
          </p>
          {subtitle && (
            <p className="mt-1 text-xs text-gray-500">{subtitle}</p>
          )}
        </div>
        <div
          className={`flex items-center justify-center w-12 h-12 rounded-xl ${color}`}
        >
          {icon}
        </div>
      </div>
    </div>
  );
}

interface GaugeProps {
  label: string;
  value: number;
  max: number;
  unit: string;
}

function RadialGauge({ label, value, max, unit }: GaugeProps) {
  const pct = max > 0 ? Math.min((value / max) * 100, 100) : 0;
  const color =
    pct > 80 ? 'text-red-500' : pct > 60 ? 'text-amber-500' : 'text-emerald-500';
  const strokeColor =
    pct > 80 ? 'stroke-red-500' : pct > 60 ? 'stroke-amber-500' : 'stroke-emerald-500';

  const radius = 36;
  const circumference = 2 * Math.PI * radius;
  const dashOffset = circumference - (pct / 100) * circumference;

  return (
    <div className="flex flex-col items-center gap-2">
      <div className="relative w-24 h-24">
        <svg className="w-24 h-24 -rotate-90" viewBox="0 0 80 80">
          <circle
            cx="40" cy="40" r={radius}
            fill="none"
            className="stroke-navy-800"
            strokeWidth="6"
          />
          <circle
            cx="40" cy="40" r={radius}
            fill="none"
            className={`${strokeColor} transition-all duration-700`}
            strokeWidth="6"
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={dashOffset}
          />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <span className={`text-lg font-display font-bold ${color}`}>
            {Math.round(pct)}%
          </span>
        </div>
      </div>
      <div className="text-center">
        <p className="text-xs text-gray-400">{label}</p>
        <p className="text-xs font-mono text-gray-500">
          {value.toLocaleString()}{unit && ` ${unit}`}
        </p>
      </div>
    </div>
  );
}

function formatTimestamp(date: Date): string {
  return date.toLocaleTimeString('en-US', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

function EventItem({ event }: { event: SSEEvent & { _ts?: Date } }) {
  const ts = (event as SSEEvent & { _ts?: Date })._ts ?? new Date();
  let parsed: Record<string, string> = {};
  try {
    parsed = JSON.parse(event.data);
  } catch {
    // data may not be JSON
  }

  const getEventStyle = (type: string) => {
    if (type.includes('created')) return 'border-emerald-500/30 text-emerald-400';
    if (type.includes('destroyed')) return 'border-red-500/30 text-red-400';
    if (type.includes('expired')) return 'border-amber-500/30 text-amber-400';
    return 'border-blue-500/30 text-blue-400';
  };

  return (
    <div
      className={`flex items-start gap-3 px-3 py-2 rounded-lg border-l-2 bg-navy-800/50 ${getEventStyle(
        event.type,
      )} animate-fade-in`}
    >
      <Zap className="w-3.5 h-3.5 mt-0.5 flex-shrink-0 opacity-60" />
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium truncate">
          {event.type === 'message' ? 'Event' : event.type}
        </p>
        {parsed.sandbox_id && (
          <p className="text-[10px] text-gray-500 font-mono truncate">
            {parsed.sandbox_id}
          </p>
        )}
      </div>
      <span className="text-[10px] text-gray-600 flex-shrink-0">
        {formatTimestamp(ts)}
      </span>
    </div>
  );
}

export default function Dashboard() {
  const { sandboxes, events } = useSandboxes();
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [metrics, setMetrics] = useState<MetricsResponse | null>(null);
  const [templateCount, setTemplateCount] = useState(0);
  const [providerCount, setProviderCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const fetchAll = async () => {
    setRefreshing(true);
    try {
      const [h, m, t, p] = await Promise.allSettled([
        getHealth(),
        getMetrics(),
        listTemplates(),
        listProviders(),
      ]);
      if (h.status === 'fulfilled') setHealth(h.value);
      if (m.status === 'fulfilled') setMetrics(m.value);
      if (t.status === 'fulfilled') setTemplateCount(t.value.length);
      if (p.status === 'fulfilled') setProviderCount(p.value.length);
    } finally {
      setRefreshing(false);
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchAll();
    const interval = setInterval(fetchAll, 10000);
    return () => clearInterval(interval);
  }, []);

  const activeSandboxes = sandboxes.filter((s) => s.status === 'running').length;

  return (
    <div className="space-y-6 max-w-7xl">
      {/* Hero banner */}
      <div className="card !p-0 overflow-hidden">
        <div className="relative px-6 py-5 flex items-center gap-5">
          <div className="absolute inset-0 bg-gradient-to-r from-primary-500/10 via-amber-500/5 to-transparent" />
          <div className="relative flex items-center justify-center w-14 h-14 rounded-2xl bg-gradient-to-br from-primary-500/20 to-amber-500/10 border border-primary-500/20">
            <Egg className="w-7 h-7 text-primary-400" />
          </div>
          <div className="relative flex-1">
            <h2 className="text-2xl font-display font-bold text-gray-100">
              ForgeVM
            </h2>
            <div className="flex items-center gap-3 mt-0.5 text-sm text-gray-400">
              {health && (
                <>
                  <span className="flex items-center gap-1.5">
                    <span className={`w-2 h-2 rounded-full ${health.status === 'ok' ? 'bg-emerald-400 animate-pulse-slow' : 'bg-red-400'}`} />
                    {health.status === 'ok' ? 'All systems operational' : 'Degraded'}
                  </span>
                  <span className="text-gray-600">&middot;</span>
                  <span className="font-mono text-xs text-gray-500">uptime {health.uptime}</span>
                  <span className="text-gray-600">&middot;</span>
                  <span className="font-mono text-xs text-gray-500">v{health.version}</span>
                </>
              )}
            </div>
          </div>
          <button
            onClick={fetchAll}
            disabled={refreshing}
            className="relative btn-ghost text-sm"
          >
            <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
          </button>
        </div>
      </div>

      {/* Stats grid */}
      {loading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
          <StatCardSkeleton />
          <StatCardSkeleton />
          <StatCardSkeleton />
          <StatCardSkeleton />
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
          <StatCard
            label="Active Sandboxes"
            value={activeSandboxes}
            icon={<Box className="w-6 h-6 text-primary-400" />}
            color="bg-primary-500/15"
            subtitle={`${sandboxes.length} total`}
            large
          />
          <StatCard
            label="Templates"
            value={templateCount}
            icon={<FileCode2 className="w-6 h-6 text-blue-400" />}
            color="bg-blue-500/15"
          />
          <StatCard
            label="Providers"
            value={providerCount}
            icon={<Server className="w-6 h-6 text-emerald-400" />}
            color="bg-emerald-500/15"
          />
        </div>
      )}

      {/* Resource gauges + Activity feed */}
      <div className="grid grid-cols-1 lg:grid-cols-5 gap-6">
        {/* Resource gauges */}
        <div className="card lg:col-span-2">
          <h3 className="text-xs font-display font-semibold text-gray-400 uppercase tracking-wider flex items-center gap-2 mb-5">
            <Cpu className="w-4 h-4" /> Resources
          </h3>
          <div className="flex justify-around">
            <RadialGauge
              label="Goroutines"
              value={metrics?.goroutines ?? 0}
              max={500}
              unit=""
            />
            <RadialGauge
              label="Memory"
              value={metrics ? Math.round(metrics.memory_alloc / 1024 / 1024) : 0}
              max={512}
              unit="MB"
            />
            <RadialGauge
              label="Sandboxes"
              value={metrics?.active_sandboxes ?? 0}
              max={Math.max(metrics?.total_sandboxes ?? 10, 10)}
              unit=""
            />
          </div>
          <div className="mt-5 pt-4 border-t border-navy-600">
            <div className="flex items-center justify-between text-sm">
              <span className="text-gray-400 flex items-center gap-1.5">
                <HardDrive className="w-4 h-4" /> Total Sandboxes
              </span>
              <span className="font-mono text-gray-300">
                {metrics?.total_sandboxes ?? 0}
              </span>
            </div>
          </div>
        </div>

        {/* Activity feed */}
        <div className="card lg:col-span-3 flex flex-col">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-xs font-display font-semibold text-gray-400 uppercase tracking-wider flex items-center gap-2">
              <Clock className="w-4 h-4" /> Activity Feed
            </h3>
            <span className="text-xs text-gray-600">
              {events.length} event{events.length !== 1 ? 's' : ''}
            </span>
          </div>
          {events.length === 0 ? (
            <div className="flex-1 flex flex-col items-center justify-center py-8 text-gray-500">
              <Activity className="w-8 h-8 mb-2 opacity-40" />
              <p className="text-sm">No events yet</p>
              <p className="text-xs text-gray-600 mt-1">
                Events appear as sandboxes are created and managed
              </p>
            </div>
          ) : (
            <div className="space-y-1.5 max-h-72 overflow-y-auto pr-1 flex-1">
              {events.slice(0, 20).map((event, i) => (
                <EventItem key={`${event.type}-${i}`} event={event} />
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Quick actions */}
      <div className="flex flex-wrap gap-3">
        <a href="/sandboxes" className="btn-primary text-sm">
          <Plus className="w-4 h-4" />
          Create Sandbox
        </a>
        <a href="/templates" className="btn-secondary text-sm">
          <FileCode2 className="w-4 h-4" />
          Manage Templates
        </a>
      </div>
    </div>
  );
}
