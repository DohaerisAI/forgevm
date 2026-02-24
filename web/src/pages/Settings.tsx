import { useState, useEffect } from 'react';
import {
  Save,
  RotateCcw,
  Sun,
  Moon,
  Clock,
  Layers,
  Shield,
  Globe,
  Palette,
  AlertCircle,
  CheckCircle2,
  Info,
} from 'lucide-react';

// Settings are persisted to localStorage since the backend may not have a settings API yet
interface AppSettings {
  defaultTtl: string;
  poolSize: number;
  authEnabled: boolean;
  authToken: string;
  serverPort: number;
  serverHost: string;
  theme: 'dark' | 'light' | 'system';
  autoRefreshInterval: number;
  maxExecHistory: number;
}

const DEFAULT_SETTINGS: AppSettings = {
  defaultTtl: '5m',
  poolSize: 5,
  authEnabled: false,
  authToken: '',
  serverPort: 7423,
  serverHost: 'localhost',
  theme: 'dark',
  autoRefreshInterval: 5,
  maxExecHistory: 100,
};

function loadSettings(): AppSettings {
  try {
    const stored = localStorage.getItem('forgevm-settings');
    if (stored) {
      return { ...DEFAULT_SETTINGS, ...JSON.parse(stored) };
    }
  } catch {
    // Ignore parse errors
  }
  return { ...DEFAULT_SETTINGS };
}

function saveSettings(settings: AppSettings) {
  localStorage.setItem('forgevm-settings', JSON.stringify(settings));
}

type SettingsSection = 'general' | 'pool' | 'auth' | 'appearance';

export default function Settings() {
  const [settings, setSettings] = useState<AppSettings>(loadSettings);
  const [saved, setSaved] = useState(false);
  const [activeSection, setActiveSection] = useState<SettingsSection>('general');
  const [hasChanges, setHasChanges] = useState(false);

  useEffect(() => {
    const stored = loadSettings();
    const changed = JSON.stringify(settings) !== JSON.stringify(stored);
    setHasChanges(changed);
  }, [settings]);

  const handleSave = () => {
    saveSettings(settings);

    // Apply theme
    if (settings.theme === 'dark') {
      document.documentElement.classList.add('dark');
      localStorage.setItem('forgevm-theme', 'dark');
    } else if (settings.theme === 'light') {
      document.documentElement.classList.remove('dark');
      localStorage.setItem('forgevm-theme', 'light');
    } else {
      // System preference
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      if (prefersDark) {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.remove('dark');
      }
      localStorage.setItem('forgevm-theme', 'system');
    }

    setSaved(true);
    setHasChanges(false);
    setTimeout(() => setSaved(false), 3000);
  };

  const handleReset = () => {
    setSettings(loadSettings());
    setHasChanges(false);
  };

  const handleRestoreDefaults = () => {
    setSettings({ ...DEFAULT_SETTINGS });
  };

  const update = <K extends keyof AppSettings>(key: K, value: AppSettings[K]) => {
    setSettings((prev) => ({ ...prev, [key]: value }));
  };

  const sections: { key: SettingsSection; label: string; icon: typeof Clock }[] = [
    { key: 'general', label: 'General', icon: Globe },
    { key: 'pool', label: 'Pool & Limits', icon: Layers },
    { key: 'auth', label: 'Authentication', icon: Shield },
    { key: 'appearance', label: 'Appearance', icon: Palette },
  ];

  return (
    <div className="space-y-6 max-w-4xl">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-display font-bold text-gray-100">Settings</h2>
          <p className="text-sm text-gray-400 mt-1">
            Configure your ForgeVM platform preferences
          </p>
        </div>
        <div className="flex items-center gap-3">
          {saved && (
            <span className="flex items-center gap-1.5 text-sm text-emerald-400 animate-fade-in">
              <CheckCircle2 className="w-4 h-4" />
              Saved
            </span>
          )}
          <button
            onClick={handleReset}
            disabled={!hasChanges}
            className="btn-ghost text-sm"
          >
            <RotateCcw className="w-4 h-4" />
            Discard
          </button>
          <button
            onClick={handleSave}
            disabled={!hasChanges}
            className="btn-primary text-sm"
          >
            <Save className="w-4 h-4" />
            Save Changes
          </button>
        </div>
      </div>

      <div className="flex flex-col lg:flex-row gap-6">
        {/* Section nav */}
        <nav className="lg:w-48 flex-shrink-0">
          <div className="flex lg:flex-col gap-1">
            {sections.map(({ key, label, icon: Icon }) => (
              <button
                key={key}
                onClick={() => setActiveSection(key)}
                className={`flex items-center gap-2.5 px-3 py-2 text-sm rounded-lg transition-colors text-left ${
                  activeSection === key
                    ? 'bg-primary-500/15 text-primary-400 font-medium'
                    : 'text-gray-400 hover:bg-navy-700 hover:text-gray-200'
                }`}
              >
                <Icon className="w-4 h-4 flex-shrink-0" />
                {label}
              </button>
            ))}
          </div>

          <div className="mt-4 pt-4 border-t border-navy-700">
            <button
              onClick={handleRestoreDefaults}
              className="text-xs text-gray-600 hover:text-gray-400 transition-colors"
            >
              Restore all defaults
            </button>
          </div>
        </nav>

        {/* Settings panel */}
        <div className="flex-1">
          {activeSection === 'general' && (
            <SettingsPanel title="General" description="Core platform configuration">
              <SettingsField
                label="Default TTL"
                description="Default time-to-live for new sandboxes (e.g. 5m, 1h, 30s)"
              >
                <input
                  type="text"
                  value={settings.defaultTtl}
                  onChange={(e) => update('defaultTtl', e.target.value)}
                  className="input w-48"
                  placeholder="5m"
                />
              </SettingsField>

              <SettingsField
                label="Server Host"
                description="Host address for the ForgeVM API server"
              >
                <input
                  type="text"
                  value={settings.serverHost}
                  onChange={(e) => update('serverHost', e.target.value)}
                  className="input w-48"
                  placeholder="localhost"
                />
              </SettingsField>

              <SettingsField
                label="Server Port"
                description="Port for the ForgeVM API server"
              >
                <input
                  type="number"
                  value={settings.serverPort}
                  onChange={(e) => update('serverPort', parseInt(e.target.value) || 7423)}
                  className="input w-32"
                  min={1}
                  max={65535}
                />
              </SettingsField>

              <SettingsField
                label="Auto-refresh Interval"
                description="How often to poll for sandbox updates (seconds)"
              >
                <input
                  type="number"
                  value={settings.autoRefreshInterval}
                  onChange={(e) =>
                    update('autoRefreshInterval', Math.max(1, parseInt(e.target.value) || 5))
                  }
                  className="input w-32"
                  min={1}
                  max={60}
                />
              </SettingsField>
            </SettingsPanel>
          )}

          {activeSection === 'pool' && (
            <SettingsPanel
              title="Pool & Limits"
              description="Resource pool sizes and execution limits"
            >
              <SettingsField
                label="Pool Size"
                description="Number of pre-warmed sandboxes to keep in the pool"
              >
                <input
                  type="number"
                  value={settings.poolSize}
                  onChange={(e) =>
                    update('poolSize', Math.max(0, parseInt(e.target.value) || 0))
                  }
                  className="input w-32"
                  min={0}
                  max={100}
                />
              </SettingsField>

              <SettingsField
                label="Max Exec History"
                description="Maximum number of execution entries to keep in the terminal history"
              >
                <input
                  type="number"
                  value={settings.maxExecHistory}
                  onChange={(e) =>
                    update('maxExecHistory', Math.max(10, parseInt(e.target.value) || 100))
                  }
                  className="input w-32"
                  min={10}
                  max={1000}
                />
              </SettingsField>

              <div className="bg-navy-800/50 rounded-lg p-3 flex items-start gap-2.5">
                <Info className="w-4 h-4 text-blue-400 mt-0.5 flex-shrink-0" />
                <p className="text-xs text-gray-500">
                  Pool sizes apply per provider. Pre-warming helps reduce sandbox startup
                  latency but uses more resources. Set to 0 to disable pre-warming.
                </p>
              </div>
            </SettingsPanel>
          )}

          {activeSection === 'auth' && (
            <SettingsPanel
              title="Authentication"
              description="API authentication settings"
            >
              <SettingsField
                label="Enable Authentication"
                description="Require API token for all requests"
              >
                <label className="relative inline-flex items-center cursor-pointer">
                  <input
                    type="checkbox"
                    checked={settings.authEnabled}
                    onChange={(e) => update('authEnabled', e.target.checked)}
                    className="sr-only peer"
                  />
                  <div
                    className="w-11 h-6 bg-navy-600 peer-focus:outline-none peer-focus:ring-2
                               peer-focus:ring-primary-500 rounded-full peer
                               peer-checked:after:translate-x-full
                               after:content-[''] after:absolute after:top-[2px] after:left-[2px]
                               after:bg-gray-400 after:rounded-full after:h-5 after:w-5
                               after:transition-all peer-checked:bg-primary-500
                               peer-checked:after:bg-white"
                  />
                </label>
              </SettingsField>

              {settings.authEnabled && (
                <SettingsField
                  label="API Token"
                  description="Bearer token used to authenticate API requests"
                >
                  <input
                    type="password"
                    value={settings.authToken}
                    onChange={(e) => update('authToken', e.target.value)}
                    className="input"
                    placeholder="Enter your API token"
                  />
                </SettingsField>
              )}

              <div className="bg-amber-500/10 border border-amber-500/20 rounded-lg p-3 flex items-start gap-2.5">
                <AlertCircle className="w-4 h-4 text-amber-400 mt-0.5 flex-shrink-0" />
                <p className="text-xs text-amber-300/80">
                  Authentication settings are stored locally in the browser.
                  Server-side auth must be configured separately in your ForgeVM config file.
                </p>
              </div>
            </SettingsPanel>
          )}

          {activeSection === 'appearance' && (
            <SettingsPanel title="Appearance" description="Theme and display preferences">
              <SettingsField
                label="Theme"
                description="Choose your preferred color scheme"
              >
                <div className="flex gap-3">
                  <ThemeButton
                    label="Dark"
                    icon={Moon}
                    active={settings.theme === 'dark'}
                    onClick={() => update('theme', 'dark')}
                  />
                  <ThemeButton
                    label="Light"
                    icon={Sun}
                    active={settings.theme === 'light'}
                    onClick={() => update('theme', 'light')}
                  />
                  <ThemeButton
                    label="System"
                    icon={Palette}
                    active={settings.theme === 'system'}
                    onClick={() => update('theme', 'system')}
                  />
                </div>
              </SettingsField>
            </SettingsPanel>
          )}
        </div>
      </div>
    </div>
  );
}

// ------------------------------------------------------------------
// Shared sub-components
// ------------------------------------------------------------------

function SettingsPanel({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <div className="card space-y-6 animate-fade-in">
      <div>
        <h3 className="text-lg font-bold text-gray-100">{title}</h3>
        <p className="text-sm text-gray-500 mt-0.5">{description}</p>
      </div>
      <div className="space-y-6">{children}</div>
    </div>
  );
}

function SettingsField({
  label,
  description,
  children,
}: {
  label: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-3">
      <div className="sm:flex-1">
        <label className="text-sm font-medium text-gray-200">{label}</label>
        {description && (
          <p className="text-xs text-gray-500 mt-0.5">{description}</p>
        )}
      </div>
      <div className="sm:flex-shrink-0">{children}</div>
    </div>
  );
}

function ThemeButton({
  label,
  icon: Icon,
  active,
  onClick,
}: {
  label: string;
  icon: typeof Sun;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`flex flex-col items-center gap-2 px-4 py-3 rounded-xl border transition-all ${
        active
          ? 'border-primary-500 bg-primary-500/10 text-primary-400'
          : 'border-navy-600 bg-navy-800 text-gray-400 hover:border-navy-400 hover:text-gray-200'
      }`}
    >
      <Icon className="w-5 h-5" />
      <span className="text-xs font-medium">{label}</span>
    </button>
  );
}
