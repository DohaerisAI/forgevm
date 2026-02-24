import { useState, useEffect, useCallback } from 'react';
import {
  Plus,
  RefreshCw,
  Trash2,
  Edit3,
  Play,
  FileCode2,
  Search,
  X,
  Loader2,
  AlertCircle,
  Clock,
  Cpu,
  Server,
} from 'lucide-react';
import {
  type Template,
  type CreateTemplateRequest,
  listTemplates,
  createTemplate,
  updateTemplate,
  deleteTemplate,
  spawnFromTemplate,
} from '../api/client';
import TemplateEditor from '../components/TemplateEditor';
import { useToast } from '../hooks/useToast';

// Convert a Template object to YAML string
function templateToYaml(tmpl: Partial<Template>): string {
  const lines: string[] = [];

  if (tmpl.name) lines.push(`name: ${tmpl.name}`);
  if (tmpl.image) lines.push(`image: ${tmpl.image}`);
  if (tmpl.provider) lines.push(`provider: ${tmpl.provider}`);
  if (tmpl.ttl) lines.push(`ttl: ${tmpl.ttl}`);
  if (tmpl.description) lines.push(`description: ${tmpl.description}`);

  if (tmpl.init_commands && tmpl.init_commands.length > 0) {
    lines.push('');
    lines.push('init_commands:');
    for (const cmd of tmpl.init_commands) {
      lines.push(`  - ${cmd}`);
    }
  }

  if (tmpl.files && Object.keys(tmpl.files).length > 0) {
    lines.push('');
    lines.push('files:');
    for (const [path, content] of Object.entries(tmpl.files)) {
      lines.push(`  ${path}: "${content.replace(/"/g, '\\"')}"`);
    }
  }

  if (tmpl.env && Object.keys(tmpl.env).length > 0) {
    lines.push('');
    lines.push('env:');
    for (const [key, value] of Object.entries(tmpl.env)) {
      lines.push(`  ${key}: ${value}`);
    }
  }

  return lines.join('\n') + '\n';
}

// Parse simple YAML back to CreateTemplateRequest
function yamlToTemplate(yaml: string): CreateTemplateRequest {
  const result: CreateTemplateRequest = { name: '', image: '' };
  const lines = yaml.split('\n');
  let currentSection: string | null = null;

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;

    // Top-level keys
    const topMatch = trimmed.match(/^(\w+):\s*(.*)$/);
    if (topMatch && !line.startsWith(' ') && !line.startsWith('\t')) {
      const [, key, value] = topMatch;

      if (key === 'init_commands' || key === 'files' || key === 'env') {
        currentSection = key;
        continue;
      }

      currentSection = null;
      const cleaned = value.replace(/^["']|["']$/g, '').trim();

      switch (key) {
        case 'name':
          result.name = cleaned;
          break;
        case 'image':
          result.image = cleaned;
          break;
        case 'provider':
          result.provider = cleaned || undefined;
          break;
        case 'ttl':
          result.ttl = cleaned || undefined;
          break;
        case 'description':
          result.description = cleaned || undefined;
          break;
      }
      continue;
    }

    // Section items
    if (currentSection === 'init_commands') {
      const listMatch = trimmed.match(/^-\s+(.*)$/);
      if (listMatch) {
        if (!result.init_commands) result.init_commands = [];
        result.init_commands.push(listMatch[1].replace(/^["']|["']$/g, ''));
      }
    } else if (currentSection === 'files') {
      const kvMatch = trimmed.match(/^(.+?):\s*(.*)$/);
      if (kvMatch) {
        if (!result.files) result.files = {};
        result.files[kvMatch[1].trim()] = kvMatch[2].replace(/^["']|["']$/g, '');
      }
    } else if (currentSection === 'env') {
      const kvMatch = trimmed.match(/^(.+?):\s*(.*)$/);
      if (kvMatch) {
        if (!result.env) result.env = {};
        result.env[kvMatch[1].trim()] = kvMatch[2].replace(/^["']|["']$/g, '');
      }
    }
  }

  return result;
}

export default function Templates() {
  const [templates, setTemplates] = useState<Template[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [refreshing, setRefreshing] = useState(false);
  const { addToast } = useToast();

  // Editor state
  const [editing, setEditing] = useState<Template | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);

  // Spawn state
  const [spawning, setSpawning] = useState<string | null>(null);
  const [spawnSuccess, setSpawnSuccess] = useState<string | null>(null);

  const fetchTemplates = useCallback(async () => {
    try {
      const data = await listTemplates();
      setTemplates(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load templates');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTemplates();
  }, [fetchTemplates]);

  const handleRefresh = async () => {
    setRefreshing(true);
    await fetchTemplates();
    setRefreshing(false);
  };

  const handleCreateNew = () => {
    setEditing({
      name: '',
      image: 'alpine:latest',
      provider: 'mock',
      ttl: '5m',
      description: '',
      init_commands: [],
      files: {},
      env: {},
    });
    setIsNew(true);
  };

  const handleEdit = (tmpl: Template) => {
    setEditing(tmpl);
    setIsNew(false);
  };

  const handleSave = async (yaml: string) => {
    setSaving(true);
    try {
      const parsed = yamlToTemplate(yaml);

      if (!parsed.name || !parsed.image) {
        throw new Error('Template must have a name and image');
      }

      if (isNew) {
        await createTemplate(parsed);
        addToast({ type: 'success', title: 'Template created', message: parsed.name });
      } else {
        await updateTemplate(parsed.name, parsed);
        addToast({ type: 'success', title: 'Template updated', message: parsed.name });
      }

      setEditing(null);
      setIsNew(false);
      await fetchTemplates();
    } catch (err) {
      addToast({ type: 'error', title: 'Failed to save template', message: err instanceof Error ? err.message : 'Unknown error' });
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (name: string) => {
    try {
      await deleteTemplate(name);
      if (editing?.name === name) {
        setEditing(null);
      }
      addToast({ type: 'success', title: 'Template deleted', message: name });
      await fetchTemplates();
    } catch (err) {
      addToast({ type: 'error', title: 'Failed to delete template', message: err instanceof Error ? err.message : 'Unknown error' });
    }
  };

  const handleSpawn = async (name: string) => {
    setSpawning(name);
    setSpawnSuccess(null);
    try {
      const sandbox = await spawnFromTemplate(name);
      setSpawnSuccess(`Sandbox ${sandbox.id} created from template "${name}"`);
      addToast({ type: 'success', title: 'Sandbox spawned', message: `${sandbox.id} from "${name}"` });
      setTimeout(() => setSpawnSuccess(null), 5000);
    } catch (err) {
      addToast({ type: 'error', title: 'Failed to spawn sandbox', message: err instanceof Error ? err.message : 'Unknown error' });
    } finally {
      setSpawning(null);
    }
  };

  const filtered = templates.filter((t) => {
    if (!search) return true;
    const q = search.toLowerCase();
    return (
      t.name.toLowerCase().includes(q) ||
      t.image.toLowerCase().includes(q) ||
      (t.description?.toLowerCase().includes(q) ?? false)
    );
  });

  // Editor view
  if (editing) {
    return (
      <div className="space-y-6 max-w-5xl">
        <div>
          <h2 className="text-2xl font-bold text-gray-100">
            {isNew ? 'Create Template' : `Edit: ${editing.name}`}
          </h2>
          <p className="text-sm text-gray-400 mt-1">
            {isNew
              ? 'Define a new template in YAML format'
              : 'Modify the template configuration below'}
          </p>
        </div>
        <TemplateEditor
          initialValue={templateToYaml(editing)}
          onSave={handleSave}
          onCancel={() => {
            setEditing(null);
            setIsNew(false);
          }}
          title={isNew ? 'New Template' : editing.name}
          saving={saving}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6 max-w-7xl">
      {/* Header */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-display font-bold text-gray-100">Templates</h2>
          <p className="text-sm text-gray-400 mt-1">
            Reusable sandbox configurations for quick deployment
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
          <button onClick={handleCreateNew} className="btn-primary text-sm">
            <Plus className="w-4 h-4" />
            New Template
          </button>
        </div>
      </div>

      {/* Success banner */}
      {spawnSuccess && (
        <div className="flex items-center gap-3 bg-emerald-500/10 border border-emerald-500/30 rounded-lg px-4 py-3 animate-fade-in">
          <Play className="w-4 h-4 text-emerald-400 flex-shrink-0" />
          <span className="text-sm text-emerald-300 flex-1">{spawnSuccess}</span>
          <button
            onClick={() => setSpawnSuccess(null)}
            className="text-emerald-500 hover:text-emerald-300"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

      {/* Search */}
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-500" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search templates..."
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

      {/* Template cards grid */}
      {loading ? (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="w-6 h-6 animate-spin text-gray-500" />
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
          <FileCode2 className="w-8 h-8 mb-3 opacity-40" />
          <p className="text-sm font-medium">
            {search ? 'No matching templates' : 'No templates defined'}
          </p>
          <p className="text-xs text-gray-600 mt-1">
            {search
              ? 'Try a different search term'
              : 'Create a template to quickly spin up preconfigured sandboxes'}
          </p>
          {!search && (
            <button onClick={handleCreateNew} className="btn-primary text-sm mt-4">
              <Plus className="w-4 h-4" />
              New Template
            </button>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filtered.map((tmpl) => (
            <TemplateCard
              key={tmpl.name}
              template={tmpl}
              onEdit={() => handleEdit(tmpl)}
              onDelete={() => handleDelete(tmpl.name)}
              onSpawn={() => handleSpawn(tmpl.name)}
              spawning={spawning === tmpl.name}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// ------------------------------------------------------------------
// Template Card component
// ------------------------------------------------------------------

interface TemplateCardProps {
  template: Template;
  onEdit: () => void;
  onDelete: () => void;
  onSpawn: () => void;
  spawning: boolean;
}

function TemplateCard({
  template,
  onEdit,
  onDelete,
  onSpawn,
  spawning,
}: TemplateCardProps) {
  const [confirmDelete, setConfirmDelete] = useState(false);

  const handleDelete = () => {
    if (confirmDelete) {
      onDelete();
      setConfirmDelete(false);
    } else {
      setConfirmDelete(true);
      setTimeout(() => setConfirmDelete(false), 3000);
    }
  };

  return (
    <div className="card !p-0 overflow-hidden animate-fade-in group">
      {/* Card header */}
      <div className="px-5 pt-5 pb-3">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-3">
            <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-blue-500/10">
              <FileCode2 className="w-5 h-5 text-blue-400" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-gray-100">{template.name}</h3>
              {template.description && (
                <p className="text-xs text-gray-500 mt-0.5 line-clamp-1">
                  {template.description}
                </p>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Card details */}
      <div className="px-5 pb-4 space-y-2">
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-400">
          <span className="flex items-center gap-1">
            <Cpu className="w-3 h-3" />
            {template.image}
          </span>
          {template.provider && (
            <span className="flex items-center gap-1">
              <Server className="w-3 h-3" />
              {template.provider}
            </span>
          )}
          {template.ttl && (
            <span className="flex items-center gap-1">
              <Clock className="w-3 h-3" />
              {template.ttl}
            </span>
          )}
        </div>

        {/* Init commands preview */}
        {template.init_commands && template.init_commands.length > 0 && (
          <div className="text-xs text-gray-600">
            {template.init_commands.length} init command{template.init_commands.length !== 1 ? 's' : ''}
          </div>
        )}

        {/* Files preview */}
        {template.files && Object.keys(template.files).length > 0 && (
          <div className="text-xs text-gray-600">
            {Object.keys(template.files).length} file{Object.keys(template.files).length !== 1 ? 's' : ''}
          </div>
        )}
      </div>

      {/* Card actions */}
      <div className="flex items-center border-t border-navy-600 divide-x divide-navy-600">
        <button
          onClick={onSpawn}
          disabled={spawning}
          className="flex items-center justify-center gap-2 flex-1 px-3 py-2.5 text-sm
                     text-emerald-400 hover:bg-emerald-500/10 transition-colors
                     disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {spawning ? (
            <Loader2 className="w-4 h-4 animate-spin" />
          ) : (
            <Play className="w-4 h-4" />
          )}
          Spawn
        </button>
        <button
          onClick={onEdit}
          className="flex items-center justify-center gap-2 flex-1 px-3 py-2.5 text-sm
                     text-blue-400 hover:bg-blue-500/10 transition-colors"
        >
          <Edit3 className="w-4 h-4" />
          Edit
        </button>
        <button
          onClick={handleDelete}
          className={`flex items-center justify-center gap-2 flex-1 px-3 py-2.5 text-sm
                     transition-colors ${
                       confirmDelete
                         ? 'text-red-300 bg-red-500/20'
                         : 'text-red-400 hover:bg-red-500/10'
                     }`}
        >
          <Trash2 className="w-4 h-4" />
          {confirmDelete ? 'Confirm' : 'Delete'}
        </button>
      </div>
    </div>
  );
}
