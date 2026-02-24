import { useState, useEffect, useCallback } from 'react';
import {
  Folder,
  FolderOpen,
  File,
  FileText,
  FileCode,
  ChevronRight,
  RefreshCw,
  Save,
  X,
  ArrowLeft,
  Loader2,
  AlertCircle,
} from 'lucide-react';
import { type FileEntry, listFiles, readFile, writeFile } from '../api/client';

interface FileBrowserProps {
  sandboxId: string;
}

function fileIcon(name: string, isDir: boolean) {
  if (isDir) return Folder;
  const ext = name.split('.').pop()?.toLowerCase() ?? '';
  if (['ts', 'tsx', 'js', 'jsx', 'go', 'py', 'rs', 'rb', 'java', 'c', 'cpp', 'h'].includes(ext)) {
    return FileCode;
  }
  if (['md', 'txt', 'log', 'json', 'yaml', 'yml', 'toml', 'xml', 'html', 'css'].includes(ext)) {
    return FileText;
  }
  return File;
}

function fileName(path: string): string {
  const parts = path.replace(/\/$/, '').split('/');
  return parts[parts.length - 1] || path;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export default function FileBrowser({ sandboxId }: FileBrowserProps) {
  const [currentPath, setCurrentPath] = useState('/workspace');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // File editor state
  const [editingFile, setEditingFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState('');
  const [originalContent, setOriginalContent] = useState('');
  const [fileLoading, setFileLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const fetchEntries = useCallback(async (path: string) => {
    setLoading(true);
    setError(null);
    try {
      const data = await listFiles(sandboxId, path);
      setEntries(data);
      setCurrentPath(path);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to list files');
      setEntries([]);
    } finally {
      setLoading(false);
    }
  }, [sandboxId]);

  useEffect(() => {
    fetchEntries(currentPath);
  }, [sandboxId]);

  const navigateTo = (path: string) => {
    setEditingFile(null);
    fetchEntries(path);
  };

  const navigateUp = () => {
    const parts = currentPath.replace(/\/$/, '').split('/');
    if (parts.length > 1) {
      parts.pop();
      navigateTo(parts.join('/') || '/');
    }
  };

  const openFile = async (path: string) => {
    setFileLoading(true);
    setSaveError(null);
    try {
      const content = await readFile(sandboxId, path);
      setFileContent(content);
      setOriginalContent(content);
      setEditingFile(path);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to read file');
    } finally {
      setFileLoading(false);
    }
  };

  const saveFile = async () => {
    if (!editingFile) return;
    setSaving(true);
    setSaveError(null);
    try {
      await writeFile(sandboxId, editingFile, fileContent);
      setOriginalContent(fileContent);
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save file');
    } finally {
      setSaving(false);
    }
  };

  const handleEntryClick = (entry: FileEntry) => {
    if (entry.is_dir) {
      navigateTo(entry.path);
    } else {
      openFile(entry.path);
    }
  };

  const breadcrumbs = currentPath.split('/').filter(Boolean);
  const hasChanges = fileContent !== originalContent;

  // File editor view
  if (editingFile) {
    return (
      <div className="flex flex-col h-96 bg-navy-950 rounded-lg border border-navy-600 overflow-hidden">
        {/* Editor header */}
        <div className="flex items-center justify-between px-4 py-2 bg-navy-900 border-b border-navy-600">
          <div className="flex items-center gap-2 min-w-0">
            <button
              onClick={() => setEditingFile(null)}
              className="text-gray-500 hover:text-gray-300 transition-colors flex-shrink-0"
              title="Back to file list"
            >
              <ArrowLeft className="w-4 h-4" />
            </button>
            <FileCode className="w-4 h-4 text-gray-400 flex-shrink-0" />
            <span className="text-sm text-gray-300 font-mono truncate">
              {fileName(editingFile)}
            </span>
            {hasChanges && (
              <span className="text-xs text-amber-400 flex-shrink-0">modified</span>
            )}
          </div>
          <div className="flex items-center gap-2">
            {saveError && (
              <span className="text-xs text-red-400 flex items-center gap-1">
                <AlertCircle className="w-3 h-3" />
                {saveError}
              </span>
            )}
            <button
              onClick={saveFile}
              disabled={!hasChanges || saving}
              className="btn-primary text-xs !px-3 !py-1"
            >
              {saving ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
              ) : (
                <Save className="w-3.5 h-3.5" />
              )}
              Save
            </button>
            <button
              onClick={() => setEditingFile(null)}
              className="text-gray-500 hover:text-gray-300 transition-colors"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Editor content */}
        {fileLoading ? (
          <div className="flex-1 flex items-center justify-center">
            <Loader2 className="w-5 h-5 animate-spin text-gray-500" />
          </div>
        ) : (
          <textarea
            value={fileContent}
            onChange={(e) => setFileContent(e.target.value)}
            className="flex-1 w-full p-4 bg-transparent text-gray-200 font-mono text-sm
                       resize-none outline-none leading-relaxed"
            spellCheck={false}
          />
        )}
      </div>
    );
  }

  // File list view
  return (
    <div className="flex flex-col h-96 bg-navy-950 rounded-lg border border-navy-600 overflow-hidden">
      {/* Browser header */}
      <div className="flex items-center justify-between px-4 py-2 bg-navy-900 border-b border-navy-600">
        <div className="flex items-center gap-2 min-w-0">
          <FolderOpen className="w-4 h-4 text-gray-400 flex-shrink-0" />
          <nav className="flex items-center gap-1 text-sm overflow-x-auto">
            <button
              onClick={() => navigateTo('/')}
              className="text-gray-500 hover:text-gray-300 transition-colors flex-shrink-0"
            >
              /
            </button>
            {breadcrumbs.map((crumb, i) => (
              <span key={i} className="flex items-center gap-1 flex-shrink-0">
                <ChevronRight className="w-3 h-3 text-gray-600" />
                <button
                  onClick={() =>
                    navigateTo('/' + breadcrumbs.slice(0, i + 1).join('/'))
                  }
                  className={`transition-colors ${
                    i === breadcrumbs.length - 1
                      ? 'text-gray-200 font-medium'
                      : 'text-gray-500 hover:text-gray-300'
                  }`}
                >
                  {crumb}
                </button>
              </span>
            ))}
          </nav>
        </div>
        <button
          onClick={() => fetchEntries(currentPath)}
          className="text-gray-500 hover:text-gray-300 transition-colors flex-shrink-0"
          title="Refresh"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {/* File list */}
      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="flex items-center justify-center py-12">
            <Loader2 className="w-5 h-5 animate-spin text-gray-500" />
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-12 text-gray-500">
            <AlertCircle className="w-6 h-6 mb-2 opacity-60" />
            <p className="text-sm">{error}</p>
          </div>
        ) : entries.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 text-gray-500">
            <Folder className="w-6 h-6 mb-2 opacity-40" />
            <p className="text-sm">Empty directory</p>
          </div>
        ) : (
          <div>
            {/* Navigate up */}
            {currentPath !== '/' && (
              <button
                onClick={navigateUp}
                className="flex items-center gap-3 w-full px-4 py-2 text-sm text-gray-400
                           hover:bg-navy-800/50 transition-colors"
              >
                <ArrowLeft className="w-4 h-4" />
                <span>..</span>
              </button>
            )}

            {/* Directories first, then files */}
            {[...entries]
              .sort((a, b) => {
                if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
                return fileName(a.path).localeCompare(fileName(b.path));
              })
              .map((entry) => {
                const name = fileName(entry.path);
                const Icon = fileIcon(name, entry.is_dir);

                return (
                  <button
                    key={entry.path}
                    onClick={() => handleEntryClick(entry)}
                    className="flex items-center gap-3 w-full px-4 py-2 text-sm
                               hover:bg-navy-800/50 transition-colors group"
                  >
                    <Icon
                      className={`w-4 h-4 flex-shrink-0 ${
                        entry.is_dir ? 'text-primary-400' : 'text-gray-500'
                      }`}
                    />
                    <span
                      className={`flex-1 text-left truncate ${
                        entry.is_dir
                          ? 'text-gray-200 font-medium'
                          : 'text-gray-300'
                      }`}
                    >
                      {name}
                    </span>
                    {!entry.is_dir && (
                      <span className="text-xs text-gray-600 font-mono flex-shrink-0">
                        {formatSize(entry.size)}
                      </span>
                    )}
                    <span className="text-xs text-gray-700 font-mono flex-shrink-0 w-24 text-right">
                      {entry.mode}
                    </span>
                  </button>
                );
              })}
          </div>
        )}
      </div>
    </div>
  );
}
