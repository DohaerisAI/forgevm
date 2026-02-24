import { useState, useEffect, useCallback } from 'react';
import {
  Server,
  RefreshCw,
  CheckCircle2,
  XCircle,
  Wifi,
  WifiOff,
  Loader2,
  AlertCircle,
  Star,
  Activity,
  ChevronDown,
  ChevronUp,
  Settings,
  Hash,
} from 'lucide-react';
import { type Provider, type ProviderDetail, listProviders, getHealth, getProviderDetail } from '../api/client';
import { ProviderCardSkeleton } from '../components/Skeleton';
import { useToast } from '../hooks/useToast';

// Known provider descriptions
const providerInfo: Record<string, { description: string; color: string }> = {
  mock: {
    description: 'Local mock provider using temp directories and os/exec. Ideal for development and testing.',
    color: 'text-amber-400',
  },
  firecracker: {
    description: 'Direct Firecracker microVM provider with custom vsock agent. Production-grade isolation.',
    color: 'text-blue-400',
  },
  e2b: {
    description: 'E2B cloud sandbox provider. Managed infrastructure with global availability.',
    color: 'text-purple-400',
  },
};

function getProviderInfo(name: string) {
  const lower = name.toLowerCase();
  for (const [key, info] of Object.entries(providerInfo)) {
    if (lower.includes(key)) return info;
  }
  return {
    description: 'Custom provider implementation.',
    color: 'text-gray-400',
  };
}

export default function Providers() {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const { addToast } = useToast();

  // Connection test state
  const [testingProvider, setTestingProvider] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<Record<string, { ok: boolean; message: string }>>({});

  const fetchProviders = useCallback(async () => {
    try {
      const data = await listProviders();
      setProviders(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load providers');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchProviders();
  }, [fetchProviders]);

  const handleRefresh = async () => {
    setRefreshing(true);
    setTestResults({});
    await fetchProviders();
    setRefreshing(false);
  };

  const handleTestConnection = async (providerName: string) => {
    setTestingProvider(providerName);
    try {
      // Test via the health endpoint (which validates backend connectivity)
      const health = await getHealth();
      const ok = health.status === 'ok';
      const message = ok
        ? `Connected successfully (uptime: ${health.uptime})`
        : `Health check returned: ${health.status}`;
      setTestResults((prev) => ({ ...prev, [providerName]: { ok, message } }));
      addToast({
        type: ok ? 'success' : 'warning',
        title: `${providerName}: ${ok ? 'Connected' : 'Unhealthy'}`,
        message,
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Connection failed';
      setTestResults((prev) => ({ ...prev, [providerName]: { ok: false, message } }));
      addToast({ type: 'error', title: `${providerName}: Connection failed`, message });
    } finally {
      setTestingProvider(null);
    }
  };

  const healthyCount = providers.filter((p) => p.healthy).length;

  return (
    <div className="space-y-6 max-w-5xl">
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-display font-bold text-gray-100">Providers</h2>
          <p className="text-sm text-gray-400 mt-1">
            Sandbox execution backends and their health status
          </p>
        </div>
        <button
          onClick={handleRefresh}
          disabled={refreshing}
          className="btn-secondary text-sm"
        >
          <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      {/* Summary bar */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="card !p-4 flex items-center gap-3">
          <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-blue-500/10">
            <Server className="w-5 h-5 text-blue-400" />
          </div>
          <div>
            <p className="text-2xl font-bold text-gray-100">{providers.length}</p>
            <p className="text-xs text-gray-500">Total Providers</p>
          </div>
        </div>
        <div className="card !p-4 flex items-center gap-3">
          <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-emerald-500/10">
            <CheckCircle2 className="w-5 h-5 text-emerald-400" />
          </div>
          <div>
            <p className="text-2xl font-bold text-gray-100">{healthyCount}</p>
            <p className="text-xs text-gray-500">Healthy</p>
          </div>
        </div>
        <div className="card !p-4 flex items-center gap-3">
          <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-red-500/10">
            <XCircle className="w-5 h-5 text-red-400" />
          </div>
          <div>
            <p className="text-2xl font-bold text-gray-100">
              {providers.length - healthyCount}
            </p>
            <p className="text-xs text-gray-500">Unhealthy</p>
          </div>
        </div>
      </div>

      {/* Provider list */}
      {loading ? (
        <div className="space-y-4">
          <ProviderCardSkeleton />
          <ProviderCardSkeleton />
        </div>
      ) : error ? (
        <div className="card flex flex-col items-center justify-center py-12 text-gray-500">
          <AlertCircle className="w-8 h-8 mb-3 opacity-60" />
          <p className="text-sm font-medium">{error}</p>
          <button onClick={handleRefresh} className="btn-secondary text-sm mt-4">
            Try Again
          </button>
        </div>
      ) : providers.length === 0 ? (
        <div className="card flex flex-col items-center justify-center py-12 text-gray-500">
          <Server className="w-8 h-8 mb-3 opacity-40" />
          <p className="text-sm font-medium">No providers configured</p>
          <p className="text-xs text-gray-600 mt-1">
            Add a provider in your ForgeVM configuration to get started
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {providers.map((provider) => (
            <ProviderCard
              key={provider.name}
              provider={provider}
              testResult={testResults[provider.name]}
              testing={testingProvider === provider.name}
              onTest={() => handleTestConnection(provider.name)}
            />
          ))}
        </div>
      )}

    </div>
  );
}

// ------------------------------------------------------------------
// Provider Card component
// ------------------------------------------------------------------

interface ProviderCardProps {
  provider: Provider;
  testResult?: { ok: boolean; message: string };
  testing: boolean;
  onTest: () => void;
}

function ProviderCard({
  provider,
  testResult,
  testing,
  onTest,
}: ProviderCardProps) {
  const info = getProviderInfo(provider.name);
  const [expanded, setExpanded] = useState(false);
  const [detail, setDetail] = useState<ProviderDetail | null>(null);
  const [loadingDetail, setLoadingDetail] = useState(false);

  const toggleExpand = async () => {
    const next = !expanded;
    setExpanded(next);
    if (next && !detail) {
      setLoadingDetail(true);
      try {
        const d = await getProviderDetail(provider.name);
        setDetail(d);
      } catch {
        // silently fail
      } finally {
        setLoadingDetail(false);
      }
    }
  };

  return (
    <div className="card !p-0 overflow-hidden animate-fade-in">
      <div
        className="flex items-center gap-4 px-5 py-4 cursor-pointer hover:bg-navy-600/30 transition-colors"
        onClick={toggleExpand}
      >
        {/* Icon */}
        <div
          className={`flex items-center justify-center w-12 h-12 rounded-xl ${
            provider.healthy ? 'bg-emerald-500/10' : 'bg-red-500/10'
          }`}
        >
          {provider.healthy ? (
            <Wifi className="w-6 h-6 text-emerald-400" />
          ) : (
            <WifiOff className="w-6 h-6 text-red-400" />
          )}
        </div>

        {/* Info */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className={`text-base font-bold ${info.color}`}>
              {provider.name}
            </h3>
            {provider.is_default && (
              <span className="flex items-center gap-1 text-xs text-amber-400 bg-amber-500/10 border border-amber-500/20 rounded-full px-2 py-0.5">
                <Star className="w-3 h-3" />
                Default
              </span>
            )}
            <span
              className={
                provider.healthy ? 'badge-success' : 'badge-danger'
              }
            >
              {provider.healthy ? 'Healthy' : 'Unhealthy'}
            </span>
          </div>
          <p className="text-xs text-gray-500 mt-1">{info.description}</p>
        </div>

        {/* Test button + expand */}
        <button
          onClick={(e) => {
            e.stopPropagation();
            onTest();
          }}
          disabled={testing}
          className="btn-secondary text-sm flex-shrink-0"
        >
          {testing ? (
            <Loader2 className="w-4 h-4 animate-spin" />
          ) : (
            <Activity className="w-4 h-4" />
          )}
          Test
        </button>
        {expanded ? (
          <ChevronUp className="w-5 h-5 text-gray-500" />
        ) : (
          <ChevronDown className="w-5 h-5 text-gray-500" />
        )}
      </div>

      {/* Test result */}
      {testResult && (
        <div
          className={`flex items-center gap-2 px-5 py-2.5 border-t text-sm ${
            testResult.ok
              ? 'border-emerald-500/20 bg-emerald-500/5 text-emerald-400'
              : 'border-red-500/20 bg-red-500/5 text-red-400'
          }`}
        >
          {testResult.ok ? (
            <CheckCircle2 className="w-4 h-4 flex-shrink-0" />
          ) : (
            <XCircle className="w-4 h-4 flex-shrink-0" />
          )}
          <span className="text-xs">{testResult.message}</span>
        </div>
      )}

      {/* Expanded config panel */}
      {expanded && (
        <div className="border-t border-navy-600 bg-navy-800/50 px-5 py-4 space-y-4">
          {loadingDetail ? (
            <div className="flex items-center justify-center py-6">
              <Loader2 className="w-5 h-5 animate-spin text-gray-500" />
            </div>
          ) : detail ? (
            <>
              {/* Stats row */}
              <div className="flex items-center gap-6 text-sm">
                <div className="flex items-center gap-2 text-gray-400">
                  <Hash className="w-4 h-4" />
                  <span className="text-gray-300 font-medium">{detail.sandbox_count}</span>
                  <span>active sandbox{detail.sandbox_count !== 1 ? 'es' : ''}</span>
                </div>
              </div>

              {/* Config table */}
              {Object.keys(detail.config).length > 0 && (
                <div>
                  <h4 className="text-xs font-semibold text-gray-400 uppercase tracking-wide flex items-center gap-1.5 mb-2">
                    <Settings className="w-3.5 h-3.5" />
                    Configuration
                  </h4>
                  <div className="bg-navy-900 rounded-lg overflow-hidden">
                    {Object.entries(detail.config).map(([key, value]) => (
                      <div
                        key={key}
                        className="flex items-center px-4 py-2 border-b border-navy-700 last:border-0"
                      >
                        <span className="text-xs text-gray-500 w-40 flex-shrink-0 font-mono">
                          {key}
                        </span>
                        <span className="text-xs text-gray-300 font-mono truncate">
                          {value}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </>
          ) : (
            <p className="text-xs text-gray-500 text-center py-4">
              Could not load provider details
            </p>
          )}
        </div>
      )}
    </div>
  );
}
