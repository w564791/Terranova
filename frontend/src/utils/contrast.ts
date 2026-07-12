/**
 * WCAG contrast helpers — pick readable foreground against any background.
 *
 * Usage:
 *   contrastText('#9a6700')           // → '#ffffff'
 *   contrastText('rgb(253, 243, 224)') // → '#1c1f24'
 *   contrastText(bg, { light: '#fff', dark: '#1c1f24' })
 *   ensureContrast(fg, bg)            // swap fg if ratio < 4.5
 */

export type RGB = { r: number; g: number; b: number };

/** Parse #rgb / #rrggbb / #rrggbbaa / rgb() / rgba() / named CSS vars won't resolve */
export function parseColor(input: string): RGB | null {
  if (!input) return null;
  const s = input.trim().toLowerCase();

  // #rgb / #rrggbb / #rrggbbaa
  if (s.startsWith('#')) {
    let h = s.slice(1);
    if (h.length === 3) {
      h = h
        .split('')
        .map((c) => c + c)
        .join('');
    }
    if (h.length === 8) h = h.slice(0, 6); // drop alpha
    if (h.length !== 6 || !/^[0-9a-f]+$/.test(h)) return null;
    return {
      r: parseInt(h.slice(0, 2), 16),
      g: parseInt(h.slice(2, 4), 16),
      b: parseInt(h.slice(4, 6), 16),
    };
  }

  // rgb() / rgba()
  const m = s.match(
    /^rgba?\(\s*([0-9.]+)\s*,\s*([0-9.]+)\s*,\s*([0-9.]+)(?:\s*,\s*[0-9.]+\s*)?\)$/
  );
  if (m) {
    return {
      r: Math.min(255, Math.max(0, Number(m[1]))),
      g: Math.min(255, Math.max(0, Number(m[2]))),
      b: Math.min(255, Math.max(0, Number(m[3]))),
    };
  }

  return null;
}

/** Relative luminance (WCAG 2.1) */
export function relativeLuminance(rgb: RGB): number {
  const channel = (c: number) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
  };
  const R = channel(rgb.r);
  const G = channel(rgb.g);
  const B = channel(rgb.b);
  return 0.2126 * R + 0.7152 * G + 0.0722 * B;
}

/** Contrast ratio between two colors (1–21) */
export function contrastRatio(a: RGB, b: RGB): number {
  const L1 = relativeLuminance(a);
  const L2 = relativeLuminance(b);
  const lighter = Math.max(L1, L2);
  const darker = Math.min(L1, L2);
  return (lighter + 0.05) / (darker + 0.05);
}

export interface ContrastTextOptions {
  /** Preferred light text (default white) */
  light?: string;
  /** Preferred dark text (default near-black ink) */
  dark?: string;
  /** Minimum ratio to prefer (default 4.5 ≈ AA normal text) */
  minRatio?: number;
}

/**
 * Choose light or dark text for a given background so contrast is maximized.
 * Falls back to dark if bg can't be parsed.
 */
export function contrastText(
  background: string,
  options: ContrastTextOptions = {}
): string {
  const light = options.light ?? '#ffffff';
  const dark = options.dark ?? '#1c1f24';
  const bg = parseColor(background);
  if (!bg) return dark;

  const lightRgb = parseColor(light)!;
  const darkRgb = parseColor(dark)!;
  const ratioLight = contrastRatio(bg, lightRgb);
  const ratioDark = contrastRatio(bg, darkRgb);

  // Prefer whichever has higher contrast; tie → dark (softer on light UIs)
  return ratioLight > ratioDark ? light : dark;
}

/**
 * If current fg on bg fails minRatio, replace with adaptive contrastText(bg).
 */
export function ensureContrast(
  foreground: string,
  background: string,
  minRatio = 4.5
): string {
  const fg = parseColor(foreground);
  const bg = parseColor(background);
  if (!fg || !bg) return contrastText(background);
  if (contrastRatio(fg, bg) >= minRatio) return foreground;
  return contrastText(background);
}

/**
 * For soft badges: given a semantic base color (e.g. #1a8a4a),
 * return { background: soft tint, color: dark ink-ish base }.
 * Soft tint ≈ mix base into white at ~12% strength.
 */
export function softBadgeColors(base: string): { background: string; color: string } {
  const rgb = parseColor(base);
  if (!rgb) {
    return { background: '#f3f4f6', color: '#1c1f24' };
  }
  // soft bg: blend base toward white
  const t = 0.12;
  const soft: RGB = {
    r: Math.round(255 * (1 - t) + rgb.r * t),
    g: Math.round(255 * (1 - t) + rgb.g * t),
    b: Math.round(255 * (1 - t) + rgb.b * t),
  };
  const bgHex = rgbToHex(soft);
  // text: prefer darkened base for brand feel, but ensure contrast
  const darkBase: RGB = {
    r: Math.round(rgb.r * 0.72),
    g: Math.round(rgb.g * 0.72),
    b: Math.round(rgb.b * 0.72),
  };
  let color = rgbToHex(darkBase);
  color = ensureContrast(color, bgHex, 4.5);
  // if still low, force adaptive
  if (contrastRatio(parseColor(color)!, soft) < 4.5) {
    color = contrastText(bgHex);
  }
  return { background: bgHex, color };
}

/**
 * Solid badge: solid base bg + auto white/dark text.
 */
export function solidBadgeColors(base: string): { background: string; color: string } {
  const bg = parseColor(base) ? base : '#1c6e8c';
  return {
    background: bg.startsWith('#') && bg.length >= 7 ? bg.slice(0, 7) : bg,
    color: contrastText(bg),
  };
}

export function rgbToHex({ r, g, b }: RGB): string {
  const h = (n: number) =>
    Math.min(255, Math.max(0, Math.round(n)))
      .toString(16)
      .padStart(2, '0');
  return `#${h(r)}${h(g)}${h(b)}`;
}

/** Resolve CSS color from a DOM element computed style (for runtime reads) */
export function readCssColor(
  el: Element | null,
  prop: 'backgroundColor' | 'color' = 'backgroundColor'
): string | null {
  if (!el || typeof window === 'undefined') return null;
  const v = getComputedStyle(el).getPropertyValue(
    prop === 'backgroundColor' ? 'background-color' : 'color'
  );
  return v || null;
}
