/**
 * Reusable Button — maps to global .btn system (styles/buttons.css)
 *
 * <Button variant="solid" tone="brand">保存</Button>
 * <Button variant="outline" tone="red">删除</Button>
 * <Button size="sm" loading>保存中</Button>
 */
import React from 'react';

export type ButtonVariant = 'solid' | 'outline' | 'ghost' | 'neutral';
export type ButtonTone = 'brand' | 'green' | 'red' | 'neutral';
export type ButtonSize = 'xs' | 'sm' | 'md' | 'lg';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  tone?: ButtonTone;
  size?: ButtonSize;
  loading?: boolean;
  block?: boolean;
  iconOnly?: boolean;
  /** @deprecated use tone="brand" variant="solid" */
  primary?: boolean;
  /** @deprecated use tone="red" */
  danger?: boolean;
}

function buildClassName(props: ButtonProps): string {
  const {
    variant = 'neutral',
    tone = 'neutral',
    size = 'md',
    loading,
    block,
    iconOnly,
    primary,
    danger,
    className,
  } = props;

  const classes = ['btn'];

  // legacy shortcuts
  let v = variant;
  let t = tone;
  if (primary) {
    v = 'solid';
    t = 'brand';
  }
  if (danger) {
    t = 'red';
    if (v === 'neutral') v = 'outline';
  }

  if (v === 'solid' || v === 'outline' || v === 'ghost') {
    classes.push(v);
  }
  // solid/outline/ghost + tone
  if (t !== 'neutral' && (v === 'solid' || v === 'outline' || v === 'ghost')) {
    classes.push(t);
  } else if (t === 'brand' && v === 'neutral') {
    // bare brand → solid brand (legacy primary feel)
    classes.push('solid', 'brand');
  } else if (t === 'red' && v === 'neutral') {
    classes.push('outline', 'red');
  } else if (t === 'green' && v === 'neutral') {
    classes.push('solid', 'green');
  }

  if (size !== 'md') classes.push(size);
  if (loading) classes.push('is-loading');
  if (block) classes.push('block');
  if (iconOnly) classes.push('icon');
  if (className) classes.push(className);

  return classes.join(' ');
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  function Button(props, ref) {
    const {
      variant: _v,
      tone: _t,
      size: _s,
      loading,
      block: _b,
      iconOnly: _i,
      primary: _p,
      danger: _d,
      disabled,
      children,
      type = 'button',
      ...rest
    } = props;

    return (
      <button
        ref={ref}
        type={type}
        className={buildClassName(props)}
        disabled={disabled || loading}
        aria-busy={loading || undefined}
        {...rest}
      >
        {children}
      </button>
    );
  }
);

export default Button;
