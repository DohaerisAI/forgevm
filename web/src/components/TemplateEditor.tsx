import { useState, useRef, useEffect, useCallback } from 'react';
import {
  Save,
  X,
  AlertCircle,
  Loader2,
  FileCode2,
  RotateCcw,
} from 'lucide-react';

interface TemplateEditorProps {
  initialValue: string;
  onSave: (value: string) => Promise<void>;
  onCancel: () => void;
  title?: string;
  saving?: boolean;
}

// Simple YAML syntax highlighting
function highlightYaml(text: string): string {
  const lines = text.split('\n');
  return lines
    .map((line) => {
      // Comments
      if (/^\s*#/.test(line)) {
        return `<span class="text-gray-500 italic">${escapeHtml(line)}</span>`;
      }

      // Key-value pairs
      const kvMatch = line.match(/^(\s*)([\w.-]+)(\s*:\s*)(.*)/);
      if (kvMatch) {
        const [, indent, key, colon, value] = kvMatch;
        let highlightedValue = escapeHtml(value);

        // Boolean values
        if (/^(true|false)$/i.test(value.trim())) {
          highlightedValue = `<span class="text-amber-400">${escapeHtml(value)}</span>`;
        }
        // Numbers
        else if (/^\d+(\.\d+)?$/.test(value.trim())) {
          highlightedValue = `<span class="text-purple-400">${escapeHtml(value)}</span>`;
        }
        // Strings with quotes
        else if (/^["'].*["']$/.test(value.trim())) {
          highlightedValue = `<span class="text-emerald-400">${escapeHtml(value)}</span>`;
        }
        // Duration-like values (e.g. 5m, 1h30m)
        else if (/^\d+[smhd]/.test(value.trim())) {
          highlightedValue = `<span class="text-blue-400">${escapeHtml(value)}</span>`;
        }
        // Other non-empty values
        else if (value.trim()) {
          highlightedValue = `<span class="text-emerald-400">${escapeHtml(value)}</span>`;
        }

        return `${escapeHtml(indent)}<span class="text-primary-400">${escapeHtml(key)}</span><span class="text-gray-500">${escapeHtml(colon)}</span>${highlightedValue}`;
      }

      // List items
      const listMatch = line.match(/^(\s*)(- )(.*)/);
      if (listMatch) {
        const [, indent, dash, value] = listMatch;
        return `${escapeHtml(indent)}<span class="text-gray-400">${escapeHtml(dash)}</span><span class="text-emerald-400">${escapeHtml(value)}</span>`;
      }

      return escapeHtml(line);
    })
    .join('\n');
}

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

const DEFAULT_TEMPLATE = `name: my-template
image: alpine:latest
provider: mock
ttl: 5m
description: A template description

init_commands:
  - echo "Hello from template"
  - apk add --no-cache curl

files:
  /workspace/hello.txt: "Hello, World!"

env:
  MY_VAR: value
`;

export default function TemplateEditor({
  initialValue,
  onSave,
  onCancel,
  title = 'Template Editor',
  saving = false,
}: TemplateEditorProps) {
  const [value, setValue] = useState(initialValue || DEFAULT_TEMPLATE);
  const [error, setError] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const highlightRef = useRef<HTMLPreElement>(null);
  const lineNumbersRef = useRef<HTMLDivElement>(null);

  const hasChanges = value !== initialValue;
  const lineCount = value.split('\n').length;

  // Sync scroll between textarea and highlight overlay
  const syncScroll = useCallback(() => {
    if (textareaRef.current && highlightRef.current && lineNumbersRef.current) {
      highlightRef.current.scrollTop = textareaRef.current.scrollTop;
      highlightRef.current.scrollLeft = textareaRef.current.scrollLeft;
      lineNumbersRef.current.scrollTop = textareaRef.current.scrollTop;
    }
  }, []);

  useEffect(() => {
    setValue(initialValue || DEFAULT_TEMPLATE);
  }, [initialValue]);

  const handleSave = async () => {
    setError(null);
    try {
      await onSave(value);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save template');
    }
  };

  const handleReset = () => {
    setValue(initialValue || DEFAULT_TEMPLATE);
    setError(null);
  };

  const handleTab = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Tab') {
      e.preventDefault();
      const textarea = textareaRef.current;
      if (!textarea) return;

      const start = textarea.selectionStart;
      const end = textarea.selectionEnd;
      const newValue = value.substring(0, start) + '  ' + value.substring(end);
      setValue(newValue);

      // Set cursor position after state update
      requestAnimationFrame(() => {
        textarea.selectionStart = start + 2;
        textarea.selectionEnd = start + 2;
      });
    }

    // Ctrl/Cmd + S to save
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
      e.preventDefault();
      handleSave();
    }
  };

  return (
    <div className="flex flex-col bg-navy-950 rounded-lg border border-navy-600 overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-2.5 bg-navy-900 border-b border-navy-600">
        <div className="flex items-center gap-2 text-sm">
          <FileCode2 className="w-4 h-4 text-gray-400" />
          <span className="text-gray-300 font-medium">{title}</span>
          {hasChanges && (
            <span className="text-xs text-amber-400">unsaved changes</span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {error && (
            <span className="text-xs text-red-400 flex items-center gap-1">
              <AlertCircle className="w-3 h-3" />
              {error}
            </span>
          )}
          <button
            onClick={handleReset}
            disabled={!hasChanges}
            className="btn-ghost text-xs !px-2 !py-1"
            title="Reset changes"
          >
            <RotateCcw className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
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
            onClick={onCancel}
            className="text-gray-500 hover:text-gray-300 transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* Editor body */}
      <div className="relative flex h-96">
        {/* Line numbers */}
        <div
          ref={lineNumbersRef}
          className="flex-shrink-0 overflow-hidden select-none bg-navy-900/50 border-r border-navy-700"
        >
          <div className="px-3 py-4 font-mono text-sm leading-relaxed">
            {Array.from({ length: lineCount }, (_, i) => (
              <div
                key={i}
                className="text-right text-gray-600 h-[1.625rem]"
              >
                {i + 1}
              </div>
            ))}
          </div>
        </div>

        {/* Editor area */}
        <div className="relative flex-1 overflow-hidden">
          {/* Syntax highlight overlay */}
          <pre
            ref={highlightRef}
            className="absolute inset-0 p-4 font-mono text-sm leading-relaxed whitespace-pre
                       overflow-auto pointer-events-none"
            aria-hidden="true"
            dangerouslySetInnerHTML={{ __html: highlightYaml(value) + '\n' }}
          />

          {/* Actual textarea (transparent text, visible caret) */}
          <textarea
            ref={textareaRef}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onScroll={syncScroll}
            onKeyDown={handleTab}
            className="absolute inset-0 w-full h-full p-4 font-mono text-sm leading-relaxed
                       bg-transparent text-transparent caret-gray-300
                       resize-none outline-none whitespace-pre overflow-auto"
            spellCheck={false}
            autoCapitalize="off"
            autoCorrect="off"
          />
        </div>
      </div>

      {/* Footer */}
      <div className="flex items-center justify-between px-4 py-1.5 bg-navy-900 border-t border-navy-600 text-xs text-gray-600">
        <span>YAML</span>
        <span>
          {lineCount} line{lineCount !== 1 ? 's' : ''} &middot; {value.length} chars
        </span>
      </div>
    </div>
  );
}
