import { useState, useRef, useEffect } from 'react';
import {
  Terminal as TerminalIcon,
  Play,
  Trash2,
  Loader2,
  ChevronRight,
  Square,
} from 'lucide-react';
import { useExecStream } from '../hooks/useExecStream';

interface TerminalProps {
  sandboxId: string;
}

export default function Terminal({ sandboxId }: TerminalProps) {
  const { history, running, execute, cancelExec, clearHistory } = useExecStream();
  const [command, setCommand] = useState('');
  const [commandHistory, setCommandHistory] = useState<string[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const outputRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [history]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const cmd = command.trim();
    if (!cmd || running) return;

    setCommandHistory((prev) => [...prev, cmd]);
    setHistoryIndex(-1);
    setCommand('');
    await execute(sandboxId, cmd);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (commandHistory.length === 0) return;
      const next = historyIndex === -1 ? commandHistory.length - 1 : Math.max(0, historyIndex - 1);
      setHistoryIndex(next);
      setCommand(commandHistory[next]);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (historyIndex === -1) return;
      const next = historyIndex + 1;
      if (next >= commandHistory.length) {
        setHistoryIndex(-1);
        setCommand('');
      } else {
        setHistoryIndex(next);
        setCommand(commandHistory[next]);
      }
    }
  };

  return (
    <div className="flex flex-col h-96 bg-navy-950 rounded-lg border border-navy-600 overflow-hidden">
      {/* Terminal header */}
      <div className="flex items-center justify-between px-4 py-2 bg-navy-900 border-b border-navy-600">
        <div className="flex items-center gap-2 text-sm text-gray-400">
          <TerminalIcon className="w-4 h-4" />
          <span>Terminal</span>
          <span className="text-xs text-gray-600 font-mono">
            {sandboxId}
          </span>
        </div>
        <button
          onClick={clearHistory}
          className="text-gray-500 hover:text-gray-300 transition-colors"
          title="Clear history"
        >
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>

      {/* Output area */}
      <div
        ref={outputRef}
        className="flex-1 overflow-y-auto p-3 font-mono text-sm space-y-3"
        onClick={() => inputRef.current?.focus()}
      >
        {history.length === 0 && (
          <div className="text-gray-600 text-xs">
            Type a command and press Enter to execute it in the sandbox.
          </div>
        )}
        {history.map((entry) => (
          <div key={entry.id} className="animate-fade-in">
            {/* Command line */}
            <div className="flex items-center gap-2 text-primary-400">
              <ChevronRight className="w-3.5 h-3.5 flex-shrink-0" />
              <span>{entry.command}</span>
            </div>

            {/* Streaming output */}
            {entry.running && entry.streamOutput.length > 0 && (
              <div className="mt-1 ml-5 space-y-0">
                {entry.streamOutput.map((chunk, i) => (
                  <pre
                    key={i}
                    className={`whitespace-pre-wrap break-all text-xs leading-relaxed ${
                      chunk.stream === 'stderr' ? 'text-red-400/80' : 'text-gray-300'
                    }`}
                  >
                    {chunk.data}
                  </pre>
                ))}
              </div>
            )}

            {/* Running indicator (no chunks yet) */}
            {entry.running && entry.streamOutput.length === 0 && (
              <div className="mt-1 ml-5 flex items-center gap-2 text-gray-500">
                <Loader2 className="w-3.5 h-3.5 animate-spin" />
                <span className="text-xs">Running...</span>
              </div>
            )}

            {/* Cancel button during execution */}
            {entry.running && (
              <div className="mt-1 ml-5">
                <button
                  onClick={cancelExec}
                  className="flex items-center gap-1.5 text-xs text-red-400 hover:text-red-300 transition-colors"
                >
                  <Square className="w-3 h-3" />
                  Cancel
                </button>
              </div>
            )}

            {entry.error && (
              <div className="mt-1 ml-5 text-red-400 text-xs">
                Error: {entry.error}
              </div>
            )}

            {entry.result && (
              <div className="mt-1 ml-5 space-y-1">
                {/* Show final output only if no stream chunks (non-streaming fallback) */}
                {entry.streamOutput.length === 0 && (
                  <>
                    {entry.result.stdout && (
                      <pre className="text-gray-300 whitespace-pre-wrap break-all text-xs leading-relaxed">
                        {entry.result.stdout}
                      </pre>
                    )}
                    {entry.result.stderr && (
                      <pre className="text-red-400/80 whitespace-pre-wrap break-all text-xs leading-relaxed">
                        {entry.result.stderr}
                      </pre>
                    )}
                  </>
                )}
                <div className="flex items-center gap-3 text-xs text-gray-600">
                  <span>
                    exit:{' '}
                    <span
                      className={
                        entry.result.exit_code === 0
                          ? 'text-emerald-500'
                          : 'text-red-400'
                      }
                    >
                      {entry.result.exit_code}
                    </span>
                  </span>
                  <span>{entry.result.duration}</span>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Input bar */}
      <form
        onSubmit={handleSubmit}
        className="flex items-center gap-2 px-3 py-2 bg-navy-900 border-t border-navy-600"
      >
        <ChevronRight className="w-4 h-4 text-primary-500 flex-shrink-0" />
        <input
          ref={inputRef}
          type="text"
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Enter command..."
          className="flex-1 bg-transparent text-gray-200 text-sm font-mono
                     placeholder-gray-600 outline-none"
          autoFocus
          disabled={running}
        />
        <button
          type="submit"
          disabled={running || !command.trim()}
          className="p-1.5 rounded-md text-primary-400 hover:bg-primary-500/10
                     disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
        >
          {running ? (
            <Loader2 className="w-4 h-4 animate-spin" />
          ) : (
            <Play className="w-4 h-4" />
          )}
        </button>
      </form>
    </div>
  );
}
