import { useState, useEffect } from 'react';
import { Routes, Route, NavLink, Navigate } from 'react-router-dom';
import {
  LayoutDashboard,
  Box,
  FileCode2,
  Server,
  FlaskConical,
  Settings as SettingsIcon,
  Menu,
  X,
  Egg,
} from 'lucide-react';
import Dashboard from './pages/Dashboard';
import Sandboxes from './pages/Sandboxes';
import Templates from './pages/Templates';
import Providers from './pages/Providers';
import Settings from './pages/Settings';
import Environments from './pages/Environments';
import { ToastProvider } from './hooks/useToast';
import ToastContainer from './components/Toast';

const navItems = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/sandboxes', label: 'Sandboxes', icon: Box },
  { to: '/templates', label: 'Templates', icon: FileCode2 },
  { to: '/environments', label: 'Environments', icon: FlaskConical },
  { to: '/providers', label: 'Providers', icon: Server },
  { to: '/settings', label: 'Settings', icon: SettingsIcon },
];

export default function App() {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [darkMode, setDarkMode] = useState(true);

  useEffect(() => {
    const stored = localStorage.getItem('forgevm-theme');
    if (stored === 'light') {
      setDarkMode(false);
      document.documentElement.classList.remove('dark');
    } else {
      setDarkMode(true);
      document.documentElement.classList.add('dark');
    }
  }, []);

  const toggleTheme = () => {
    const next = !darkMode;
    setDarkMode(next);
    if (next) {
      document.documentElement.classList.add('dark');
      localStorage.setItem('forgevm-theme', 'dark');
    } else {
      document.documentElement.classList.remove('dark');
      localStorage.setItem('forgevm-theme', 'light');
    }
  };

  return (
    <ToastProvider>
    <div className="flex h-screen overflow-hidden">
      {/* Mobile overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/50 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside
        className={`
          fixed inset-y-0 left-0 z-40 w-64 transform transition-transform duration-200 ease-in-out
          lg:relative lg:translate-x-0
          ${sidebarOpen ? 'translate-x-0' : '-translate-x-full'}
          bg-navy-900 dark:bg-navy-900 border-r border-navy-700
          flex flex-col
        `}
      >
        {/* Logo */}
        <div className="flex items-center gap-3 px-6 py-5 border-b border-navy-700">
          <div className="flex items-center justify-center w-9 h-9 rounded-lg bg-gradient-to-br from-primary-500/25 to-amber-500/15">
            <Egg className="w-5 h-5 text-primary-400" />
          </div>
          <div>
            <h1 className="text-lg font-display font-bold text-gray-100 tracking-tight">
              ForgeVM
            </h1>
            <p className="text-[10px] text-gray-500 uppercase tracking-widest">
              MicroVM Platform
            </p>
          </div>
          <button
            className="ml-auto lg:hidden text-gray-400 hover:text-gray-200"
            onClick={() => setSidebarOpen(false)}
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Navigation */}
        <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
          {navItems.map(({ to, label, icon: Icon }) => (
            <NavLink
              key={to}
              to={to}
              onClick={() => setSidebarOpen(false)}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors duration-150 ${
                  isActive
                    ? 'bg-primary-500/15 text-primary-400'
                    : 'text-gray-400 hover:bg-navy-700 hover:text-gray-200'
                }`
              }
            >
              <Icon className="w-5 h-5 flex-shrink-0" />
              {label}
            </NavLink>
          ))}
        </nav>

        {/* Footer */}
        <div className="px-4 py-4 border-t border-navy-700">
          <div className="text-xs text-gray-600">
            ForgeVM v0.1.0
          </div>
        </div>
      </aside>

      {/* Main content */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Top bar */}
        <header className="flex items-center gap-4 px-4 lg:px-6 py-3 border-b border-navy-700 dark:border-navy-700 bg-navy-800/80 dark:bg-navy-800/80 backdrop-blur-sm">
          <button
            className="lg:hidden text-gray-400 hover:text-gray-200"
            onClick={() => setSidebarOpen(true)}
          >
            <Menu className="w-6 h-6" />
          </button>
          <div className="flex-1" />
          <button
            onClick={toggleTheme}
            className="btn-ghost text-sm"
            title={darkMode ? 'Switch to light mode' : 'Switch to dark mode'}
          >
            {darkMode ? 'Light' : 'Dark'}
          </button>
        </header>

        {/* Page content */}
        <main className="flex-1 overflow-y-auto p-4 lg:p-6">
          <Routes>
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/sandboxes" element={<Sandboxes />} />
            <Route path="/templates" element={<Templates />} />
            <Route path="/environments" element={<Environments />} />
            <Route path="/providers" element={<Providers />} />
            <Route path="/settings" element={<Settings />} />
          </Routes>
        </main>
      </div>
    </div>
    <ToastContainer />
    </ToastProvider>
  );
}
