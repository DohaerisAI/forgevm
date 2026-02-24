/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#fff7ed',
          100: '#ffedd5',
          200: '#fed7aa',
          300: '#fdba74',
          400: '#fb923c',
          500: '#FF6600',
          600: '#ea580c',
          700: '#c2410c',
          800: '#9a3412',
          900: '#7c2d12',
          950: '#431407',
        },
        navy: {
          50: '#e8e8f0',
          100: '#c5c5d6',
          200: '#9e9eb8',
          300: '#77779a',
          400: '#555580',
          500: '#363660',
          600: '#2a2a4a',
          700: '#222240',
          800: '#1a1a2e',
          900: '#12121f',
          950: '#0a0a14',
        },
      },
      fontFamily: {
        sans: [
          'IBM Plex Sans',
          'system-ui',
          '-apple-system',
          'sans-serif',
        ],
        display: [
          'Space Grotesk',
          'system-ui',
          'sans-serif',
        ],
        mono: [
          'JetBrains Mono',
          'Fira Code',
          'Cascadia Code',
          'monospace',
        ],
      },
    },
  },
  plugins: [],
};
