import { useState, useEffect } from 'react';
import {
  Plus,
  RefreshCw,
  Search,
  Terminal as TerminalIcon,
  FolderOpen,
  ScrollText,
  X,
  Loader2,
  Box,
  AlertCircle,
  Trash2,
  CheckSquare,
  Square,
  Zap,
} from 'lucide-react';
import { useSandboxes } from '../hooks/useSandboxes';
import { useToast } from '../hooks/useToast';
import {
  type CreateSandboxRequest,
  type Template,
  type SnapshotSummary,
  createSandbox,
  destroySandbox,
  extendSandboxTTL,
  listTemplates,
  listSnapshots,
} from '../api/client';
import SandboxCard from '../components/SandboxCard';
import Terminal from '../components/Terminal';
import FileBrowser from '../components/FileBrowser';
import LogViewer from '../components/LogViewer';
import { SandboxCardSkeleton } from '../components/Skeleton';

type DetailTab = 'terminal' | 'files' | 'console';

export default function Sandboxes() {
  const { sandboxes, loading, error, refresh } = useSandboxes();
  const { addToast } = useToast();
  const [refreshing, setRefreshing] = useState(false);
  const [search, setSearch] = useState('');
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<DetailTab>('terminal');

  // Create dialog state
  const [showCreate, setShowCreate] = useState(false);
  const [createForm, setCreateForm] = useState<CreateSandboxRequest>({
    image: 'alpine:latest',
    ttl: '5m',
    provider: '',
  });
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  // Template state for create dialog
  const [templates, setTemplates] = useState<Template[]>([]);
  const [selectedTemplate, setSelectedTemplate] = useState('');

  // Snapshot state for create dialog
  const [snapshots, setSnapshots] = useState<SnapshotSummary[]>([]);

  // Fetch templates and snapshots when create dialog opens
  useEffect(() => {
    if (showCreate) {
      listTemplates().then(setTemplates).catch(() => setTemplates([]));
      listSnapshots().then(setSnapshots).catch(() => setSnapshots([]));
    }
  }, [showCreate]);

  const handleTemplateSelect = (templateName: string) => {
    setSelectedTemplate(templateName);
    if (!templateName) {
      setCreateForm({ image: 'alpine:latest', ttl: '5m', provider: '' });
      return;
    }
    const t = templates.find((t) => t.name === templateName);
    if (t) {
      setCreateForm({
        image: t.image,
        ttl: t.ttl || '5m',
        provider: t.provider || '',
      });
    }
  };

  // Destroy in-progress state
  const [, setDestroyingId] = useState<string | null>(null);

  // Bulk selection
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [showBulkConfirm, setShowBulkConfirm] = useState(false);
  const [bulkDestroying, setBulkDestroying] = useState(false);

  const handleRefresh = async () => {
    setRefreshing(true);
    await refresh();
    setRefreshing(false);
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreating(true);
    setCreateError(null);
    try {
      const req: CreateSandboxRequest = {
        image: createForm.image,
        ttl: createForm.ttl || undefined,
        provider: createForm.provider || undefined,
      };
      const sb = await createSandbox(req);
      setShowCreate(false);
      setCreateForm({ image: 'alpine:latest', ttl: '5m', provider: '' });
      setSelectedTemplate('');
      addToast({ type: 'success', title: 'Sandbox created', message: `ID: ${sb.id}` });
      await refresh();
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to create sandbox';
      setCreateError(msg);
      addToast({ type: 'error', title: 'Failed to create sandbox', message: msg });
    } finally {
      setCreating(false);
    }
  };

  const handleExtend = async (id: string) => {
    try {
      await extendSandboxTTL(id, '30m');
      addToast({ type: 'success', title: 'TTL extended', message: `+30 minutes for ${id}` });
      await refresh();
    } catch (err) {
      addToast({ type: 'error', title: 'Failed to extend TTL', message: err instanceof Error ? err.message : 'Unknown error' });
    }
  };

  const handleDestroy = async (id: string) => {
    setDestroyingId(id);
    try {
      await destroySandbox(id);
      if (expandedId === id) setExpandedId(null);
      addToast({ type: 'success', title: 'Sandbox destroyed', message: `ID: ${id}` });
      await refresh();
    } catch (err) {
      addToast({ type: 'error', title: 'Failed to destroy sandbox', message: err instanceof Error ? err.message : 'Unknown error' });
    } finally {
      setDestroyingId(null);
    }
  };

  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
    setActiveTab('terminal');
  };

  const toggleSelect = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const selectAll = () => {
    setSelectedIds(new Set(filtered.map((s) => s.id)));
  };

  const deselectAll = () => {
    setSelectedIds(new Set());
  };

  const handleBulkDestroy = async () => {
    setBulkDestroying(true);
    const ids = Array.from(selectedIds);
    const results = await Promise.allSettled(ids.map((id) => destroySandbox(id)));

    let successCount = 0;
    let failCount = 0;
    results.forEach((r) => {
      if (r.status === 'fulfilled') successCount++;
      else failCount++;
    });

    if (successCount > 0) {
      addToast({ type: 'success', title: `Destroyed ${successCount} sandbox${successCount > 1 ? 'es' : ''}` });
    }
    if (failCount > 0) {
      addToast({ type: 'error', title: `Failed to destroy ${failCount} sandbox${failCount > 1 ? 'es' : ''}` });
    }

    setSelectedIds(new Set());
    setShowBulkConfirm(false);
    setBulkDestroying(false);
    if (expandedId && ids.includes(expandedId)) setExpandedId(null);
    await refresh();
  };

  // Filter sandboxes
  const filtered = sandboxes.filter((s) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      s.id.toLowerCase().includes(q) ||
      s.image.toLowerCase().includes(q) ||
      s.provider.toLowerCase().includes(q) ||
      s.status.toLowerCase().includes(q)
    );
  });

  const selectionActive = selectedIds.size > 0;
  const allSelected = filtered.length > 0 && selectedIds.size === filtered.length;

  const tabItems: { key: DetailTab; label: string; icon: typeof TerminalIcon }[] = [
    { key: 'terminal', label: 'Terminal', icon: TerminalIcon },
    { key: 'files', label: 'Files', icon: FolderOpen },
    { key: 'console', label: 'Console', icon: ScrollText },
  ];

  return (
    <div className="space-y-6 max-w-7xl">
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-display font-bold text-gray-100">Sandboxes</h2>
          <p className="text-sm text-gray-400 mt-1">
            Manage your running microVM sandboxes
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={handleRefresh}
            disabled={refreshing}
            className="btn-secondary text-sm"
          >
            <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
            Refresh
          </button>
          <button
            onClick={() => setShowCreate(true)}
            className="btn-primary text-sm"
          >
            <Plus className="w-4 h-4" />
            Create Sandbox
          </button>
        </div>
      </div>

      {/* Search bar */}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-500" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by ID, image, provider, or status..."
          className="input pl-10"
        />
        {search && (
          <button
            onClick={() => setSearch('')}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 hover:text-gray-300"
          >
            <X className="w-4 h-4" />
          </button>
        )}
      </div>

      {/* Bulk action bar */}
      {selectionActive && (
        <div className="flex items-center gap-4 px-4 py-3 bg-navy-700 border border-primary-500/30 rounded-lg animate-slide-up">
          <span className="text-sm text-gray-300 font-medium">
            {selectedIds.size} selected
          </span>
          <button
            onClick={allSelected ? deselectAll : selectAll}
            className="btn-ghost text-xs !px-2 !py-1"
          >
            {allSelected ? (
              <><Square className="w-3.5 h-3.5" /> Deselect All</>
            ) : (
              <><CheckSquare className="w-3.5 h-3.5" /> Select All</>
            )}
          </button>
          <div className="flex-1" />
          <button onClick={deselectAll} className="btn-ghost text-xs !px-2 !py-1">
            Cancel
          </button>
          <button
            onClick={() => setShowBulkConfirm(true)}
            className="btn-danger text-sm !py-1.5"
          >
            <Trash2 className="w-4 h-4" />
            Destroy {selectedIds.size}
          </button>
        </div>
      )}

      {/* Sandbox list */}
      {loading && sandboxes.length === 0 ? (
        <div className="space-y-3">
          <SandboxCardSkeleton />
          <SandboxCardSkeleton />
          <SandboxCardSkeleton />
        </div>
      ) : error ? (
        <div className="card flex flex-col items-center justify-center py-12 text-gray-500">
          <AlertCircle className="w-8 h-8 mb-3 opacity-60" />
          <p className="text-sm font-medium">{error}</p>
          <button onClick={handleRefresh} className="btn-secondary text-sm mt-4">
            Try Again
          </button>
        </div>
      ) : filtered.length === 0 ? (
        <div className="card flex flex-col items-center justify-center py-12 text-gray-500">
          <Box className="w-8 h-8 mb-3 opacity-40" />
          <p className="text-sm font-medium">
            {search ? 'No matching sandboxes' : 'No sandboxes running'}
          </p>
          <p className="text-xs text-gray-600 mt-1">
            {search
              ? 'Try a different search term'
              : 'Create a sandbox to get started'}
          </p>
          {!search && (
            <button
              onClick={() => setShowCreate(true)}
              className="btn-primary text-sm mt-4"
            >
              <Plus className="w-4 h-4" />
              Create Sandbox
            </button>
          )}
        </div>
      ) : (
        <div className="space-y-3">
          <div className="text-sm text-gray-500">
            {filtered.length} sandbox{filtered.length !== 1 ? 'es' : ''}
            {search && ` matching "${search}"`}
          </div>
          {filtered.map((sandbox) => (
            <SandboxCard
              key={sandbox.id}
              sandbox={sandbox}
              expanded={expandedId === sandbox.id}
              onToggle={() => toggleExpand(sandbox.id)}
              onDestroy={handleDestroy}
              onExtend={handleExtend}
              selectable
              selected={selectedIds.has(sandbox.id)}
              onSelect={toggleSelect}
            >
              {/* Tab bar */}
              <div className="flex items-center gap-1 mb-4 border-b border-navy-600 -mx-4 px-4">
                {tabItems.map(({ key, label, icon: Icon }) => (
                  <button
                    key={key}
                    onClick={() => setActiveTab(key)}
                    className={`flex items-center gap-2 px-3 py-2 text-sm font-medium border-b-2 transition-colors ${
                      activeTab === key
                        ? 'border-primary-500 text-primary-400'
                        : 'border-transparent text-gray-500 hover:text-gray-300'
                    }`}
                  >
                    <Icon className="w-4 h-4" />
                    {label}
                  </button>
                ))}
              </div>

              {/* Tab content */}
              {activeTab === 'terminal' && (
                <Terminal sandboxId={sandbox.id} />
              )}
              {activeTab === 'files' && (
                <FileBrowser sandboxId={sandbox.id} />
              )}
              {activeTab === 'console' && (
                <LogViewer sandboxId={sandbox.id} />
              )}
            </SandboxCard>
          ))}
        </div>
      )}

      {/* Bulk destroy confirmation */}
      {showBulkConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => !bulkDestroying && setShowBulkConfirm(false)}
          />
          <div className="relative card w-full max-w-md animate-slide-up">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-bold text-gray-100">Destroy {selectedIds.size} Sandbox{selectedIds.size > 1 ? 'es' : ''}?</h3>
              <button
                onClick={() => setShowBulkConfirm(false)}
                disabled={bulkDestroying}
                className="text-gray-500 hover:text-gray-300 transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="space-y-3 mb-5">
              <p className="text-sm text-gray-400">
                This will permanently destroy the following sandboxes:
              </p>
              <div className="max-h-40 overflow-y-auto space-y-1 bg-navy-900 rounded-lg p-3">
                {Array.from(selectedIds).map((id) => (
                  <div key={id} className="text-xs font-mono text-gray-300">{id}</div>
                ))}
              </div>
              <div className="flex items-center gap-2 text-xs text-amber-400 bg-amber-500/10 rounded-lg px-3 py-2">
                <AlertCircle className="w-4 h-4 flex-shrink-0" />
                This action cannot be undone.
              </div>
            </div>

            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowBulkConfirm(false)}
                disabled={bulkDestroying}
                className="btn-secondary text-sm"
              >
                Cancel
              </button>
              <button
                onClick={handleBulkDestroy}
                disabled={bulkDestroying}
                className="btn-danger text-sm"
              >
                {bulkDestroying ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Trash2 className="w-4 h-4" />
                )}
                Destroy {selectedIds.size}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Create sandbox dialog */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={() => setShowCreate(false)}
          />
          <div className="relative card w-full max-w-lg animate-slide-up">
            <div className="flex items-center justify-between mb-6">
              <h3 className="text-lg font-bold text-gray-100">Create Sandbox</h3>
              <button
                onClick={() => setShowCreate(false)}
                className="text-gray-500 hover:text-gray-300 transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleCreate} className="space-y-4">
              {snapshots.length > 0 && (
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1.5">
                    <Zap className="w-3.5 h-3.5 inline-block mr-1 text-amber-400" />
                    Quick Launch
                  </label>
                  <div className="flex flex-wrap gap-2">
                    {snapshots.map((snap) => (
                      <button
                        key={`${snap.provider}-${snap.image}`}
                        type="button"
                        onClick={() => {
                          setCreateForm((prev) => ({ ...prev, image: snap.image }));
                          setSelectedTemplate('');
                        }}
                        className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border text-sm transition-colors ${
                          createForm.image === snap.image
                            ? 'border-amber-500/50 bg-amber-500/10 text-amber-300'
                            : 'border-navy-600 bg-navy-800 text-gray-300 hover:border-gray-500'
                        }`}
                      >
                        <Zap className="w-3.5 h-3.5 text-amber-400" />
                        {snap.image}
                        <span className="text-[10px] text-gray-500 ml-1">
                          {new Date(snap.created_at).toLocaleDateString()}
                        </span>
                      </button>
                    ))}
                  </div>
                  <p className="text-xs text-gray-500 mt-1">
                    Snapshot-ready — launches via restore instead of cold boot
                  </p>
                </div>
              )}

              {templates.length > 0 && (
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1.5">
                    From Template
                  </label>
                  <select
                    value={selectedTemplate}
                    onChange={(e) => handleTemplateSelect(e.target.value)}
                    className="input"
                  >
                    <option value="">— Custom image —</option>
                    {templates.map((t) => (
                      <option key={t.name} value={t.name}>
                        {t.name} — {t.image}
                      </option>
                    ))}
                  </select>
                </div>
              )}

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1.5">
                  Image
                </label>
                <input
                  type="text"
                  value={createForm.image}
                  onChange={(e) => {
                    setCreateForm((prev) => ({ ...prev, image: e.target.value }));
                    setSelectedTemplate('');
                  }}
                  placeholder="e.g. alpine:latest, ubuntu:22.04"
                  className="input"
                  required
                />
                <p className="text-xs text-gray-500 mt-1">
                  Any Docker image works — e.g. python:3.12, node:20, golang:1.22
                </p>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1.5">
                    TTL
                  </label>
                  <input
                    type="text"
                    value={createForm.ttl}
                    onChange={(e) =>
                      setCreateForm((prev) => ({ ...prev, ttl: e.target.value }))
                    }
                    placeholder="e.g. 5m, 1h"
                    className="input"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-1.5">
                    Provider
                  </label>
                  <input
                    type="text"
                    value={createForm.provider}
                    onChange={(e) =>
                      setCreateForm((prev) => ({
                        ...prev,
                        provider: e.target.value,
                      }))
                    }
                    placeholder="default"
                    className="input"
                  />
                </div>
              </div>

              {createError && (
                <div className="flex items-center gap-2 text-sm text-red-400 bg-red-500/10 rounded-lg px-3 py-2">
                  <AlertCircle className="w-4 h-4 flex-shrink-0" />
                  {createError}
                </div>
              )}

              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreate(false)}
                  className="btn-secondary text-sm"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creating || !createForm.image}
                  className="btn-primary text-sm"
                >
                  {creating ? (
                    <Loader2 className="w-4 h-4 animate-spin" />
                  ) : (
                    <Plus className="w-4 h-4" />
                  )}
                  Create
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}

