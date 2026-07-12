/**
 * Badge with automatic text contrast against its background.
 *
 * <ContrastBadge background="#9a6700">After Create</ContrastBadge>
 * <ContrastBadge tone="success" variant="soft">After Create</ContrastBadge>
 */
import React, { useMemo } from 'react';
import { softBadgeColors, solidBadgeColors, contrastText } from '../../utils/contrast';
import { colors } from '../../styles/tokens';

export type BadgeTone = 'brand' | 'success' | 'danger' | 'warning' | 'neutral';
export type BadgeVariant = 'soft' | 'solid';

const TONE_BASE: Record<BadgeTone, string> = {
  brand: colors.brand,
  success: colors.green,
  danger: colors.red,
  warning: colors.amber,
  neutral: colors.ink2,
};

export interface ContrastBadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  /** Explicit bg hex/rgb — highest priority */
  background?: string;
  /** Semantic tone when background not set */
  tone?: BadgeTone;
  /** soft (default) = tinted bg + dark text; solid = fill + auto light/dark text */
  variant?: BadgeVariant;
  children: React.ReactNode;
}

export const ContrastBadge: React.FC<ContrastBadgeProps> = ({
  background,
  tone = 'neutral',
  variant = 'soft',
  children,
  style,
  className,
  ...rest
}) => {
  const palette = useMemo(() => {
    if (background) {
      if (variant === 'solid') return solidBadgeColors(background);
      // soft from explicit color: mix + adaptive
      return softBadgeColors(background);
    }
    const base = TONE_BASE[tone];
    return variant === 'solid' ? solidBadgeColors(base) : softBadgeColors(base);
  }, [background, tone, variant]);

  // final safety: if caller overrode background via style, still force readable color
  const color = palette.color || contrastText(palette.background);

  return (
    <span
      className={className}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        padding: '2px 8px',
        borderRadius: 4,
        fontSize: 11,
        fontWeight: 600,
        lineHeight: 1.4,
        whiteSpace: 'nowrap',
        background: palette.background,
        color,
        ...style,
      }}
      {...rest}
    >
      {children}
    </span>
  );
};

export default ContrastBadge;
