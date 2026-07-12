/**
 * Design tokens for JS / inline styles.
 * Keep in sync with theme.css (demo/button.html · brand E #1c6e8c).
 *
 * Prefer CSS variables in stylesheets; use this module only when
 * you need a concrete color in JS (charts, canvas, style props).
 */
export const colors = {
  bg: '#f7f8fa',
  surface: '#ffffff',
  surface2: '#f3f4f6',
  surface3: '#eceef1',
  line: '#e3e6ea',
  line2: '#d4d8de',
  ink: '#1c1f24',
  ink2: '#5c6470',
  ink3: '#8a929e',
  inkFaint: '#aeb4bd',

  brand: '#1c6e8c',
  brandInk: '#135269',
  brand700: '#0d4152',
  brandSoft: '#e2f0f5',
  brandLine: '#b3d3de',
  brand100: '#e2f0f5',
  brand200: '#c6e0e8',
  brand300: '#8fc1d0',
  brand400: '#4e97ab',
  brand500: '#1c6e8c',
  brand600: '#135269',

  green: '#1a8a4a',
  greenHover: '#157a42',
  greenActive: '#116b39',
  greenSoft: '#e8f6ee',
  greenLine: '#bfe3cd',

  red: '#c8302f',
  redHover: '#b3282a',
  redActive: '#9f2326',
  redSoft: '#fbecec',
  redLine: '#e6b3b3',

  amber: '#9a6700',
  amberHover: '#825700',
  amberSoft: '#fdf3e0',
  amberLine: '#f0d78c',

  /** Link / progress only — not for buttons */
  blue: '#3858e9',
  blueSoft: '#eef1fe',
} as const;

export const rings = {
  brand: 'rgba(28, 110, 140, 0.26)',
  green: 'rgba(26, 138, 74, 0.26)',
  red: 'rgba(200, 48, 47, 0.26)',
  amber: 'rgba(154, 103, 0, 0.28)',
  blue: 'rgba(56, 88, 233, 0.26)',
} as const;

/** Status map for toast / notice / banner */
export const statusColors = {
  success: colors.green,
  error: colors.red,
  danger: colors.red,
  warning: colors.amber,
  info: colors.brand,
} as const;

export type StatusType = keyof typeof statusColors;

export function statusColor(type: StatusType): string {
  return statusColors[type] ?? colors.brand;
}
