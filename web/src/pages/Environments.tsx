import { type Dispatch, type SetStateAction, useCallback, useEffect, useMemo, useState } from 'react';
import {
  Box,
  Plus,
  RefreshCw,
  Sparkles,
  Wrench,
  Cloud,
  HardDrive,
  Trash2,
  ClipboardCopy,
  Loader2,
  CheckCircle2,
  AlertCircle,
  Info,
  ChevronDown,
} from 'lucide-react';
import {
  type EnvironmentBuild,
  type EnvironmentBuildListItem,
  type EnvironmentSpec,
  type EnvironmentSpawnConfig,
  type RegistryConnection,
  createEnvironmentSpec,
  listEnvironmentSpecs,
  listEnvironmentBuilds,
  startEnvironmentBuild,
  getEnvironmentBuild,
  cancelEnvironmentBuild,
  getEnvironmentSpawnConfig,
  getEnvironmentSuggestions,
  saveRegistryConnection,
  listRegistryConnections,
  deleteRegistryConnection,
} from '../api/client';
import { useToast } from '../hooks/useToast';

const POLL_INTERVAL_MS = 3000;
const HISTORY_LIMIT = 5;
let transientOwnerId = '';

const FINAL_BUILD_STATES = new Set(['ready', 'failed', 'canceled']);

function isFinalStatus(status: string): boolean {
  return FINAL_BUILD_STATES.has(status);
}

function splitPackageInput(raw: string): string[] {
  return raw
    .split(',')
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

function parseBuildLog(log: string): string[] {
  return log
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

export default function Environments() {
  const { addToast } = useToast();
  const [ownerId, setOwnerId] = useState(() => transientOwnerId);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [lastRefreshedAt, setLastRefreshedAt] = useState<Date | null>(null);

  const [specs, setSpecs] = useState<EnvironmentSpec[]>([]);
  const [connections, setConnections] = useState<RegistryConnection[]>([]);
  const [buildIds, setBuildIds] = useState<string[]>([]);
  const [buildsById, setBuildsById] = useState<Record<string, EnvironmentBuild>>({});
  const [specByBuildId, setSpecByBuildId] = useState<Record<string, EnvironmentSpec>>({});
  const [spawnConfigByBuildId, setSpawnConfigByBuildId] = useState<Record<string, EnvironmentSpawnConfig>>({});

  const [creatingSpec, setCreatingSpec] = useState(false);
  const [startingBuild, setStartingBuild] = useState(false);
  const [cancelingBuildId, setCancelingBuildId] = useState<string | null>(null);
  const [savingConnection, setSavingConnection] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [expandedBuildLogs, setExpandedBuildLogs] = useState<Set<string>>(new Set());

  const [selectedSpecId, setSelectedSpecId] = useState('');
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [loadingSuggestions, setLoadingSuggestions] = useState(false);

  const [newSpecName, setNewSpecName] = useState('');
  const [newBaseImage, setNewBaseImage] = useState('python:3.12-slim');
  const [newPythonVersion, setNewPythonVersion] = useState('3.12');
  const [pythonPackages, setPythonPackages] = useState<string[]>([]);
  const [aptPackages, setAptPackages] = useState<string[]>([]);
  const [pythonPkgInput, setPythonPkgInput] = useState('');
  const [aptPkgInput, setAptPkgInput] = useState('');

  const [buildTargets, setBuildTargets] = useState<Array<'local' | 'ghcr' | 'dockerhub'>>(['local']);

  const [connectionProvider, setConnectionProvider] = useState<'ghcr' | 'dockerhub'>('ghcr');
  const [connectionUsername, setConnectionUsername] = useState('');
  const [connectionSecretRef, setConnectionSecretRef] = useState('');
  const [connectionDefault, setConnectionDefault] = useState(false);
  const [deletingConnectionId, setDeletingConnectionId] = useState<string | null>(null);

  const builds = useMemo(
    () => buildIds.map((id) => buildsById[id]).filter((item): item is EnvironmentBuild => Boolean(item)),
    [buildIds, buildsById],
  );
  const activeBuilds = useMemo(
    () => builds.filter((build) => !isFinalStatus(build.status)),
    [builds],
  );
  const historyBuilds = useMemo(
    () => builds.filter((build) => isFinalStatus(build.status)),
    [builds],
  );
  const visibleBuilds = useMemo(
    () => {
      if (showHistory) return builds.slice(0, HISTORY_LIMIT);
      const latestCompleted = historyBuilds.length > 0 ? historyBuilds[0] : null;
      if (!latestCompleted) return activeBuilds;
      if (activeBuilds.some((b) => b.id === latestCompleted.id)) return activeBuilds;
      return [...activeBuilds, latestCompleted];
    },
    [showHistory, builds, activeBuilds, historyBuilds],
  );

  const refreshAll = useCallback(async (showSpinner = false) => {
    if (!ownerId.trim()) return;
    if (showSpinner) setRefreshing(true);
    try {
      const [specRows, connectionRows] = await Promise.all([
        listEnvironmentSpecs(ownerId.trim()),
        listRegistryConnections(ownerId.trim()),
      ]);
      setSpecs(specRows);
      setConnections(connectionRows);
      const buildRows = await listEnvironmentBuilds(ownerId.trim(), 40);
      setBuildIds(buildRows.map((row: EnvironmentBuildListItem) => row.build.id));
      setBuildsById(
        buildRows.reduce<Record<string, EnvironmentBuild>>((acc, row) => {
          acc[row.build.id] = row.build;
          return acc;
        }, {}),
      );
      setSpecByBuildId(
        buildRows.reduce<Record<string, EnvironmentSpec>>((acc, row) => {
          acc[row.build.id] = row.spec;
          return acc;
        }, {}),
      );
      setLastRefreshedAt(new Date());
      if (!selectedSpecId && specRows.length > 0) {
        setSelectedSpecId(specRows[0].id);
      }
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to load environment data',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    } finally {
      setLoading(false);
      if (showSpinner) setRefreshing(false);
    }
  }, [addToast, ownerId, selectedSpecId]);

  useEffect(() => {
    setLoading(true);
    void refreshAll();
  }, [ownerId, refreshAll]);

  useEffect(() => {
    transientOwnerId = ownerId;
  }, [ownerId]);

  useEffect(() => {
    if (buildIds.length === 0) return undefined;
    const timer = window.setInterval(() => {
      const pending = buildIds.filter((id) => {
        const row = buildsById[id];
        return row && !isFinalStatus(row.status);
      });
      if (pending.length === 0) return;

      void Promise.all(pending.map((id) => getEnvironmentBuild(id)))
        .then((rows) => {
          setBuildsById((prev) => {
            const next = { ...prev };
            for (const row of rows) next[row.id] = row;
            return next;
          });
        })
        .catch(() => {
          // Keep quiet for polling failures.
        });
    }, POLL_INTERVAL_MS);

    return () => window.clearInterval(timer);
  }, [buildIds, buildsById]);

  const addPackages = (
    value: string,
    setter: Dispatch<SetStateAction<string[]>>,
    resetInput: () => void,
  ) => {
    const items = splitPackageInput(value);
    if (items.length === 0) return;
    setter((prev) => {
      const seen = new Set(prev);
      const next = [...prev];
      for (const item of items) {
        if (seen.has(item)) continue;
        seen.add(item);
        next.push(item);
      }
      return next;
    });
    resetInput();
  };

  const removePackage = (
    value: string,
    setter: Dispatch<SetStateAction<string[]>>,
  ) => {
    setter((prev) => prev.filter((item) => item !== value));
  };

  const handleCreateSpec = async (e: React.FormEvent) => {
    e.preventDefault();
    const pendingPython = splitPackageInput(pythonPkgInput);
    const pendingApt = splitPackageInput(aptPkgInput);
    const finalPython = Array.from(new Set([...pythonPackages, ...pendingPython]));
    const finalApt = Array.from(new Set([...aptPackages, ...pendingApt]));

    if (!ownerId.trim() || !newSpecName.trim() || !newBaseImage.trim()) {
      addToast({
        type: 'warning',
        title: 'Missing fields',
        message: 'Owner, spec name, and base image are required.',
      });
      return;
    }

    setCreatingSpec(true);
    try {
      const created = await createEnvironmentSpec({
        owner_id: ownerId.trim(),
        name: newSpecName.trim(),
        base_image: newBaseImage.trim(),
        python_packages: finalPython,
        apt_packages: finalApt,
        python_version: newPythonVersion.trim() || undefined,
      });
      setSpecs((prev) => [created, ...prev]);
      setSelectedSpecId(created.id);
      setNewSpecName('');
      setPythonPackages([]);
      setAptPackages([]);
      setPythonPkgInput('');
      setAptPkgInput('');
      addToast({ type: 'success', title: 'Environment spec created', message: created.name });
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to create spec',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    } finally {
      setCreatingSpec(false);
    }
  };

  const handleGetSuggestions = async () => {
    if (!selectedSpecId) return;
    setLoadingSuggestions(true);
    try {
      const resp = await getEnvironmentSuggestions(selectedSpecId);
      setSuggestions(resp.suggestions);
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to get suggestions',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    } finally {
      setLoadingSuggestions(false);
    }
  };

  const handleStartBuild = async () => {
    if (!selectedSpecId) {
      addToast({ type: 'warning', title: 'Select a spec first' });
      return;
    }
    if (buildTargets.length === 0) {
      addToast({ type: 'warning', title: 'Choose at least one target' });
      return;
    }
    if (buildTargets.includes('ghcr') && !connections.some((c) => c.provider === 'ghcr' && c.username.trim() !== '')) {
      addToast({
        type: 'warning',
        title: 'GHCR connection required',
        message: 'Add a valid GHCR username and token in Registry Connections first.',
      });
      return;
    }
    if (buildTargets.includes('dockerhub') && !connections.some((c) => c.provider === 'dockerhub' && c.username.trim() !== '')) {
      addToast({
        type: 'warning',
        title: 'Docker Hub connection required',
        message: 'Add a valid Docker Hub username and token in Registry Connections first.',
      });
      return;
    }

    setStartingBuild(true);
    try {
      const build = await startEnvironmentBuild({
        spec_id: selectedSpecId,
        targets: buildTargets,
      });
      setBuildsById((prev) => ({ ...prev, [build.id]: build }));
      setBuildIds((prev) => [build.id, ...prev]);
      const spec = specs.find((row) => row.id === selectedSpecId);
      if (spec) {
        setSpecByBuildId((prev) => ({ ...prev, [build.id]: spec }));
      }
      addToast({ type: 'success', title: 'Build queued', message: build.id });
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to start build',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    } finally {
      setStartingBuild(false);
    }
  };

  const handleCancelBuild = async (buildId: string) => {
    setCancelingBuildId(buildId);
    try {
      const updated = await cancelEnvironmentBuild(buildId);
      setBuildsById((prev) => ({ ...prev, [buildId]: updated }));
      addToast({ type: 'info', title: 'Build canceled', message: buildId });
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to cancel build',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    } finally {
      setCancelingBuildId(null);
    }
  };

  const handleFetchSpawnConfig = async (buildId: string) => {
    try {
      const cfg = await getEnvironmentSpawnConfig(buildId);
      setSpawnConfigByBuildId((prev) => ({ ...prev, [buildId]: cfg }));
      addToast({
        type: 'success',
        title: 'Spawn config ready',
        message: `Use image: ${cfg.image}`,
      });
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to fetch spawn config',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    }
  };

  const handleCopySpawnImage = async (buildId: string) => {
    const image = spawnConfigByBuildId[buildId]?.image;
    if (!image) return;
    try {
      await navigator.clipboard.writeText(image);
      addToast({ type: 'success', title: 'Copied image ref', message: image });
    } catch {
      addToast({ type: 'warning', title: 'Clipboard blocked', message: image });
    }
  };

  const handleHideSpawnConfig = (buildId: string) => {
    setSpawnConfigByBuildId((prev) => {
      if (!prev[buildId]) return prev;
      const next = { ...prev };
      delete next[buildId];
      return next;
    });
  };

  const toggleBuildLog = (buildId: string) => {
    setExpandedBuildLogs((prev) => {
      const next = new Set(prev);
      if (next.has(buildId)) {
        next.delete(buildId);
      } else {
        next.add(buildId);
      }
      return next;
    });
  };

  const handleSaveRegistryConnection = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!ownerId.trim() || !connectionUsername.trim() || !connectionSecretRef.trim()) {
      addToast({
        type: 'warning',
        title: 'Missing fields',
        message: 'Owner, username, and token/secret are required.',
      });
      return;
    }

    setSavingConnection(true);
    try {
      const row = await saveRegistryConnection({
        owner_id: ownerId.trim(),
        provider: connectionProvider,
        username: connectionUsername.trim(),
        secret_ref: connectionSecretRef.trim(),
        is_default: connectionDefault,
      });
      setConnections((prev) => [row, ...prev]);
      setConnectionUsername('');
      setConnectionSecretRef('');
      setConnectionDefault(false);
      addToast({ type: 'success', title: 'Registry connection saved' });
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to save registry connection',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    } finally {
      setSavingConnection(false);
    }
  };

  const handleDeleteConnection = async (id: string) => {
    setDeletingConnectionId(id);
    try {
      await deleteRegistryConnection(id);
      setConnections((prev) => prev.filter((row) => row.id !== id));
      addToast({ type: 'success', title: 'Registry connection deleted' });
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Failed to delete registry connection',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    } finally {
      setDeletingConnectionId(null);
    }
  };

  const handleHardRefresh = () => {
    window.location.reload();
  };

  return (
    <div className="space-y-6 max-w-7xl">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-display font-bold text-gray-100">Environments</h2>
          <p className="text-sm text-gray-400 mt-1">
            Define package stacks, build custom sandbox images, and manually switch image refs.
          </p>
          {lastRefreshedAt && (
            <p className="text-[11px] text-gray-500 mt-1">
              Last refreshed: {lastRefreshedAt.toLocaleTimeString()}
            </p>
          )}
        </div>
        <button onClick={handleHardRefresh} disabled={refreshing} className="btn-secondary text-sm">
          <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
          Refresh
        </button>
      </div>

      <div className="card !p-4">
        <label className="text-xs text-gray-500 uppercase tracking-widest">Workspace ID</label>
        <input
          value={ownerId}
          onChange={(e) => setOwnerId(e.target.value)}
          placeholder="e.g. customer-a (groups specs/builds/connections)"
          className="input mt-2 max-w-md"
        />
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <form className="card space-y-4" onSubmit={handleCreateSpec}>
          <div className="flex items-center gap-2">
            <Sparkles className="w-4 h-4 text-primary-400" />
            <h3 className="font-semibold text-gray-100">Create Environment Spec</h3>
          </div>
          <input
            className="input"
            placeholder="Spec name (e.g. data-science)"
            value={newSpecName}
            onChange={(e) => setNewSpecName(e.target.value)}
          />
          <input
            className="input"
            placeholder="Base image (e.g. python:3.12-slim)"
            value={newBaseImage}
            onChange={(e) => setNewBaseImage(e.target.value)}
          />
          <input
            className="input"
            placeholder="Python version (optional)"
            value={newPythonVersion}
            onChange={(e) => setNewPythonVersion(e.target.value)}
          />

          <PackageInput
            title="Python Packages"
            hint="Add pip packages. Example: pandas,numpy,scipy"
            packages={pythonPackages}
            input={pythonPkgInput}
            setInput={setPythonPkgInput}
            onAdd={() => addPackages(pythonPkgInput, setPythonPackages, () => setPythonPkgInput(''))}
            onRemove={(value) => removePackage(value, setPythonPackages)}
          />

          <PackageInput
            title="APT Packages"
            hint="Add apt packages. Example: ffmpeg,curl,git"
            packages={aptPackages}
            input={aptPkgInput}
            setInput={setAptPkgInput}
            onAdd={() => addPackages(aptPkgInput, setAptPackages, () => setAptPkgInput(''))}
            onRemove={(value) => removePackage(value, setAptPackages)}
          />

          <button type="submit" className="btn-primary text-sm" disabled={creatingSpec}>
            {creatingSpec ? <Loader2 className="w-4 h-4 animate-spin" /> : <Plus className="w-4 h-4" />}
            Create Spec
          </button>
        </form>

        <div className="card space-y-4">
          <div className="flex items-center gap-2">
            <Wrench className="w-4 h-4 text-blue-400" />
            <h3 className="font-semibold text-gray-100">Build Custom Image</h3>
          </div>
          <div className="relative">
            <select
              className="input appearance-none pr-10"
              value={selectedSpecId}
              onChange={(e) => setSelectedSpecId(e.target.value)}
            >
              <option value="">Select a spec</option>
              {specs.map((spec) => (
                <option key={spec.id} value={spec.id}>
                  {spec.name} ({spec.base_image})
                </option>
              ))}
            </select>
            <span className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-gray-500">
              <ChevronDown className="w-4 h-4" />
            </span>
          </div>
          <div className="flex items-center gap-2 flex-wrap">
            {(['local', 'ghcr', 'dockerhub'] as const).map((target) => (
              <label
                key={target}
                className="inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-navy-500 bg-navy-800 text-sm"
              >
                <input
                  type="checkbox"
                  checked={buildTargets.includes(target)}
                  onChange={(e) => {
                    if (e.target.checked) {
                      setBuildTargets((prev) => Array.from(new Set([...prev, target])));
                    } else {
                      setBuildTargets((prev) => prev.filter((row) => row !== target));
                    }
                  }}
                />
                {target}
              </label>
            ))}
          </div>

          <div className="flex gap-2">
            <button
              type="button"
              onClick={handleStartBuild}
              className="btn-primary text-sm"
              disabled={startingBuild}
            >
              {startingBuild ? <Loader2 className="w-4 h-4 animate-spin" /> : <Box className="w-4 h-4" />}
              Start Build
            </button>
            <button
              type="button"
              onClick={handleGetSuggestions}
              className="btn-ghost text-sm"
              disabled={!selectedSpecId || loadingSuggestions}
            >
              {loadingSuggestions ? <Loader2 className="w-4 h-4 animate-spin" /> : <Sparkles className="w-4 h-4" />}
              Suggestions
            </button>
          </div>

          {suggestions.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {suggestions.map((pkg) => (
                <button
                  key={pkg}
                  type="button"
                  className="badge-info cursor-pointer"
                  onClick={() =>
                    setPythonPackages((prev) => (prev.includes(pkg) ? prev : [...prev, pkg]))
                  }
                >
                  + {pkg}
                </button>
              ))}
            </div>
          )}

          {loading ? (
            <p className="text-sm text-gray-500">Loading specs...</p>
          ) : specs.length === 0 ? (
            <div className="flex items-center gap-2 text-sm text-amber-400">
              <AlertCircle className="w-4 h-4" />
              Create a spec first, then start a build.
            </div>
          ) : null}
        </div>
      </div>

      <div className="card space-y-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <Cloud className="w-4 h-4 text-emerald-400" />
            <h3 className="font-semibold text-gray-100">Builds</h3>
          </div>
          <button
            type="button"
            className="btn-ghost text-xs !px-3 !py-1.5"
            onClick={() => setShowHistory((prev) => !prev)}
          >
            {showHistory ? 'Hide history' : `Show history (${Math.min(historyBuilds.length, HISTORY_LIMIT)})`}
          </button>
        </div>
        {visibleBuilds.length === 0 ? (
          <p className="text-sm text-gray-500">
            {showHistory ? 'No builds found for this owner.' : 'No active builds right now.'}
          </p>
        ) : (
          <div className="space-y-3">
            {visibleBuilds.map((build) => {
              const spawnCfg = spawnConfigByBuildId[build.id];
              const ready = isFinalStatus(build.status);
              const spec = specByBuildId[build.id];
              const pipSummary = spec?.python_packages ?? [];
              const aptSummary = spec?.apt_packages ?? [];
              const logLines = parseBuildLog(build.log);
              const logSummary = logLines.length > 0 ? logLines[logLines.length - 1] : 'No build log yet.';
              const logExpanded = expandedBuildLogs.has(build.id);
              const isLatestCompleted = !showHistory && isFinalStatus(build.status) && historyBuilds[0]?.id === build.id;
              return (
                <div key={build.id} className="rounded-lg border border-navy-600 bg-navy-800 p-4 space-y-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-xs text-gray-500">Build {build.id}</span>
                    <span className={build.status === 'ready' ? 'badge-success' : build.status === 'failed' ? 'badge-danger' : 'badge-warning'}>
                      {build.status}
                    </span>
                    {build.current_step && <span className="badge-neutral">{build.current_step}</span>}
                    {isLatestCompleted && <span className="badge-info">latest completed</span>}
                  </div>

                  {spec && (
                    <div className="text-xs text-gray-400">
                      <span className="text-gray-500">Spec:</span> {spec.name} ({spec.base_image})
                    </div>
                  )}
                  {(pipSummary.length > 0 || aptSummary.length > 0) && (
                    <div className="flex flex-wrap gap-2">
                      {pipSummary.slice(0, 6).map((pkg) => (
                        <span key={`pip-${build.id}-${pkg}`} className="badge-info">
                          pip:{pkg}
                        </span>
                      ))}
                      {aptSummary.slice(0, 4).map((pkg) => (
                        <span key={`apt-${build.id}-${pkg}`} className="badge-neutral">
                          apt:{pkg}
                        </span>
                      ))}
                      {(pipSummary.length > 6 || aptSummary.length > 4) && (
                        <span className="badge-neutral">
                          +{Math.max(0, pipSummary.length - 6) + Math.max(0, aptSummary.length - 4)} more
                        </span>
                      )}
                    </div>
                  )}

                  <div className="rounded-md border border-navy-600/70 bg-navy-900/60 px-3 py-2 space-y-1">
                    <div className="text-xs text-gray-300 break-all">
                      {logSummary}
                    </div>
                    <button
                      type="button"
                      className="text-[11px] text-gray-500 hover:text-gray-300"
                      onClick={() => toggleBuildLog(build.id)}
                    >
                      {logExpanded ? 'Hide details' : `Show details (${logLines.length})`}
                    </button>
                    {logExpanded && (
                      <div className="text-[11px] text-gray-400 space-y-1 pt-1 border-t border-navy-700">
                        {logLines.map((line, idx) => (
                          <div key={`${build.id}-log-${idx}`} className="break-all">
                            {line}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  <div className="flex flex-wrap gap-2">
                    <button
                      type="button"
                      className="btn-secondary text-xs !px-3 !py-1.5"
                      onClick={() => void handleFetchSpawnConfig(build.id)}
                    >
                      <HardDrive className="w-3.5 h-3.5" />
                      Get Spawn Config
                    </button>
                    {!ready ? (
                      <button
                        type="button"
                        className="btn-ghost text-xs !px-3 !py-1.5"
                        disabled={cancelingBuildId === build.id}
                        onClick={() => void handleCancelBuild(build.id)}
                      >
                        {cancelingBuildId === build.id ? (
                          <Loader2 className="w-3.5 h-3.5 animate-spin" />
                        ) : null}
                        Cancel
                      </button>
                    ) : (
                      <span className="badge-neutral">Finalized</span>
                    )}
                  </div>

                  {spawnCfg && (
                    <div className="rounded border border-emerald-500/30 bg-emerald-500/5 p-3 text-xs space-y-2">
                      <div className="flex items-center justify-between gap-2 text-emerald-300">
                        <div className="flex items-center gap-2">
                        <CheckCircle2 className="w-4 h-4" />
                        Spawn image ready for manual selection
                        </div>
                        <button
                          type="button"
                          className="text-emerald-300/80 hover:text-emerald-200 text-[11px]"
                          onClick={() => handleHideSpawnConfig(build.id)}
                        >
                          Hide
                        </button>
                      </div>
                      <div className="text-gray-200 break-all">{spawnCfg.image}</div>
                      <button
                        type="button"
                        className="btn-ghost text-xs !px-3 !py-1"
                        onClick={() => void handleCopySpawnImage(build.id)}
                      >
                        <ClipboardCopy className="w-3.5 h-3.5" />
                        Copy image ref
                      </button>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>

      <div className="card space-y-4">
        <div className="flex items-center gap-2">
          <Cloud className="w-4 h-4 text-amber-400" />
          <h3 className="font-semibold text-gray-100">Registry Connections</h3>
        </div>
        <form className="grid grid-cols-1 md:grid-cols-5 gap-3" onSubmit={handleSaveRegistryConnection}>
          <div className="relative">
            <select
              className="input appearance-none pr-16"
              value={connectionProvider}
              onChange={(e) => setConnectionProvider(e.target.value as 'ghcr' | 'dockerhub')}
            >
              <option value="ghcr">ghcr</option>
              <option value="dockerhub">dockerhub</option>
            </select>
            <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-500">
              <ChevronDown className="w-4 h-4" />
            </span>
            <div className="absolute right-9 top-1/2 -translate-y-1/2 group">
              <button
                type="button"
                className="inline-flex h-6 w-6 items-center justify-center text-gray-500 hover:text-gray-200 leading-none"
                aria-label="Registry setup help"
              >
                <Info className="w-4 h-4" />
              </button>
              <div className="hidden group-hover:block absolute z-10 top-8 left-0 w-80 rounded-lg border border-gray-200 bg-white p-3 text-xs text-gray-700 shadow-xl dark:border-navy-500 dark:bg-navy-900 dark:text-gray-300">
                {connectionProvider === 'ghcr' ? (
                  <div className="space-y-1">
                    <div className="font-medium text-gray-900 dark:text-gray-100">GHCR setup</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Username:</span> your GitHub username/org.</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Token:</span> GitHub PAT with <code>write:packages</code>.</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Push path:</span> <code>ghcr.io/&lt;username&gt;/forgevm-env:&lt;tag&gt;</code>.</div>
                  </div>
                ) : (
                  <div className="space-y-1">
                    <div className="font-medium text-gray-900 dark:text-gray-100">Docker Hub setup</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Username:</span> your Docker Hub username.</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Token:</span> Docker Hub access token.</div>
                    <div><span className="text-gray-500 dark:text-gray-400">Push path:</span> <code>docker.io/&lt;username&gt;/forgevm-env:&lt;tag&gt;</code>.</div>
                  </div>
                )}
              </div>
            </div>
          </div>
          <input
            className="input md:col-span-2"
            placeholder="Registry username"
            value={connectionUsername}
            onChange={(e) => setConnectionUsername(e.target.value)}
          />
          <input
            className="input md:col-span-2"
            placeholder="Token / secret ref"
            value={connectionSecretRef}
            onChange={(e) => setConnectionSecretRef(e.target.value)}
          />
          <label className="inline-flex items-center gap-2 text-sm text-gray-300 md:col-span-3">
            <input
              type="checkbox"
              checked={connectionDefault}
              onChange={(e) => setConnectionDefault(e.target.checked)}
            />
            Set as default for this provider
          </label>
          <button type="submit" className="btn-primary text-sm md:col-span-2" disabled={savingConnection}>
            {savingConnection ? <Loader2 className="w-4 h-4 animate-spin" /> : <Cloud className="w-4 h-4" />}
            Save Connection
          </button>
        </form>

        {connections.length === 0 ? (
          <p className="text-sm text-gray-500">No registry connections saved for this owner.</p>
        ) : (
          <div className="space-y-2">
            {connections.map((row) => (
              <div key={row.id} className="rounded-lg border border-navy-600 bg-navy-800 p-3 flex flex-wrap items-center gap-3">
                <span className="badge-info">{row.provider}</span>
                <span className="text-sm text-gray-200">{row.username}</span>
                {row.is_default && <span className="badge-success">default</span>}
                <button
                  type="button"
                  className="btn-ghost text-xs !px-2 !py-1 ml-auto"
                  disabled={deletingConnectionId === row.id}
                  onClick={() => void handleDeleteConnection(row.id)}
                >
                  {deletingConnectionId === row.id ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Trash2 className="w-3.5 h-3.5" />}
                  Delete
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

interface PackageInputProps {
  title: string;
  hint: string;
  packages: string[];
  input: string;
  setInput: (value: string) => void;
  onAdd: () => void;
  onRemove: (value: string) => void;
}

function PackageInput(props: PackageInputProps) {
  const { title, hint, packages, input, setInput, onAdd, onRemove } = props;
  return (
    <div className="space-y-2">
      <div className="text-sm text-gray-300">{title}</div>
      <div className="flex gap-2">
        <input
          className="input"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder={hint}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              onAdd();
            }
          }}
        />
        <button type="button" className="btn-secondary text-sm" onClick={onAdd}>
          Add
        </button>
      </div>
      {packages.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {packages.map((pkg) => (
            <button
              key={pkg}
              type="button"
              className="badge-neutral cursor-pointer"
              onClick={() => onRemove(pkg)}
              title="Click to remove"
            >
              {pkg}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
