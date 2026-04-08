import type { Config } from 'tailwindcss';

export default {
  darkMode: 'class',
  content: [
    './src/**/*.{ts,tsx}',
    './components/**/*.{ts,tsx}',
  ],
  theme: {
    extend: {
      colors: {
        // Surface hierarchy
        'surface': '#f8f9fa',
        'surface-bright': '#f8f9fa',
        'background': '#f8f9fa',
        'surface-dim': '#d9dadb',
        'surface-container-lowest': '#ffffff',
        'surface-container-low': '#f3f4f5',
        'surface-container': '#edeeef',
        'surface-container-high': '#e7e8e9',
        'surface-container-highest': '#e1e3e4',
        'surface-variant': '#e1e3e4',

        // Primary
        'primary': '#004ac6',
        'primary-container': '#2563eb',
        'on-primary': '#ffffff',
        'on-primary-container': '#eeefff',
        'primary-fixed': '#dbe1ff',
        'primary-fixed-dim': '#b4c5ff',

        // Secondary
        'secondary': '#495c95',
        'secondary-container': '#acbfff',
        'on-secondary': '#ffffff',
        'on-secondary-container': '#394c84',
        'secondary-fixed': '#dbe1ff',
        'secondary-fixed-dim': '#b4c5ff',

        // Outline
        'outline': '#737686',
        'outline-variant': '#c3c6d7',

        // Surface text
        'on-surface': '#191c1d',
        'on-surface-variant': '#434655',
        'on-background': '#191c1d',

        // Error
        'error': '#ba1a1a',
        'on-error': '#ffffff',
        'error-container': '#ffdad6',
        'on-error-container': '#93000a',

        // Tertiary
        'tertiary': '#943700',
        'tertiary-container': '#bc4800',
        'on-tertiary': '#ffffff',
        'on-tertiary-container': '#ffede6',
        'tertiary-fixed': '#ffdbcd',
        'tertiary-fixed-dim': '#ffb596',

        // Inverse
        'inverse-surface': '#2e3132',
        'inverse-on-surface': '#f0f1f2',
        'inverse-primary': '#b4c5ff',
      },
      borderRadius: {
        'none': '0px',
        'DEFAULT': '0px',
        'sm': '0px',
        'md': '0px',
        'lg': '0px',
        'xl': '0px',
        'full': '0px',
      },
      fontFamily: {
        'sans': ['Geist', 'sans-serif'],
        'mono': ['JetBrains Mono', 'monospace'],
      },
      boxShadow: {
        'ambient': '0 10px 30px rgba(25, 28, 29, 0.04)',
        'none': 'none',
      },
    },
  },
  plugins: [],
} satisfies Config;
