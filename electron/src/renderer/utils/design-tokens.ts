export const COLORS = {
  primary: '#004ac6',
  primaryContainer: '#2563eb',
  onPrimary: '#ffffff',
  surface: '#f8f9fa',
  surfaceContainerLowest: '#ffffff',
  surfaceContainerLow: '#f3f4f5',
  surfaceContainer: '#edeeef',
  surfaceContainerHigh: '#e7e8e9',
  surfaceContainerHighest: '#e1e3e4',
  outline: '#737686',
  outlineVariant: '#c3c6d7',
  onSurface: '#191c1d',
  onSurfaceVariant: '#434655',
  error: '#ba1a1a',
  onError: '#ffffff',
} as const;

export const FONTS = {
  sans: "'IBM Plex Mono', monospace",
  mono: "'IBM Plex Mono', monospace",
} as const;

export const SHADOWS = {
  ambient: '0 10px 30px rgba(25, 28, 29, 0.04)',
} as const;

export const BREAKPOINTS = {
  xs: '0px',
  sm: '640px',
  md: '768px',
  lg: '1024px',
  xl: '1280px',
  '2xl': '1536px',
} as const;
