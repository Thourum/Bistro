/* @ds-bundle: {"format":3,"namespace":"BistroDesignSystem_8e22c3","components":[{"name":"Avatar","sourcePath":"components/core/Avatar.jsx"},{"name":"Badge","sourcePath":"components/core/Badge.jsx"},{"name":"Button","sourcePath":"components/core/Button.jsx"},{"name":"Eyebrow","sourcePath":"components/core/Eyebrow.jsx"},{"name":"IconButton","sourcePath":"components/core/IconButton.jsx"},{"name":"Kbd","sourcePath":"components/core/Kbd.jsx"},{"name":"StatusDot","sourcePath":"components/core/StatusDot.jsx"},{"name":"CommandRow","sourcePath":"components/feedback/CommandRow.jsx"},{"name":"LineFocusChip","sourcePath":"components/feedback/LineFocusChip.jsx"},{"name":"Toast","sourcePath":"components/feedback/Toast.jsx"},{"name":"Checkbox","sourcePath":"components/forms/Checkbox.jsx"},{"name":"Input","sourcePath":"components/forms/Input.jsx"},{"name":"SegmentedControl","sourcePath":"components/forms/SegmentedControl.jsx"},{"name":"Switch","sourcePath":"components/forms/Switch.jsx"},{"name":"Panel","sourcePath":"components/layout/Panel.jsx"},{"name":"SystemBar","sourcePath":"components/layout/SystemBar.jsx"},{"name":"Toolbar","sourcePath":"components/layout/Toolbar.jsx"},{"name":"CommentCard","sourcePath":"components/review/CommentCard.jsx"},{"name":"DiffLine","sourcePath":"components/review/DiffLine.jsx"},{"name":"PRRow","sourcePath":"components/review/PRRow.jsx"}],"sourceHashes":{"components/core/Avatar.jsx":"282ef8e1cc95","components/core/Badge.jsx":"566aff4f23b5","components/core/Button.jsx":"0dcb00784eb9","components/core/Eyebrow.jsx":"a33c1e1d7f80","components/core/IconButton.jsx":"cadb64e7baa2","components/core/Kbd.jsx":"fdca86d3808e","components/core/StatusDot.jsx":"cc31145dc89a","components/feedback/CommandRow.jsx":"c41a50471663","components/feedback/LineFocusChip.jsx":"ab6eb3a2cf59","components/feedback/Toast.jsx":"82e86d611ee3","components/forms/Checkbox.jsx":"8b788a2894c6","components/forms/Input.jsx":"576dd91d90fe","components/forms/SegmentedControl.jsx":"3b12d619c103","components/forms/Switch.jsx":"3dd4a183797f","components/layout/Panel.jsx":"99c74b34e1bc","components/layout/SystemBar.jsx":"9c9df36fbf66","components/layout/Toolbar.jsx":"4f4a32303336","components/review/CommentCard.jsx":"5e590afb35d2","components/review/DiffLine.jsx":"c635fea8170d","components/review/PRRow.jsx":"3a3fecdf0062","ui_kits/bistro-app/DiffScreen.jsx":"843a0ca46518","ui_kits/bistro-app/InboxScreen.jsx":"dec0071f79e2","ui_kits/bistro-app/LoginScreen.jsx":"6ff39ab077b6","ui_kits/bistro-app/Overlays.jsx":"d75df51a2659","ui_kits/bistro-app/OverviewScreen.jsx":"416f285e002a","ui_kits/bistro-app/SettingsScreen.jsx":"ed7653914918","ui_kits/bistro-app/data.js":"1960925ab0d4"},"inlinedExternals":[],"unexposedExports":[]} */

(() => {

const __ds_ns = (window.BistroDesignSystem_8e22c3 = window.BistroDesignSystem_8e22c3 || {});

const __ds_scope = {};

(__ds_ns.__errors = __ds_ns.__errors || []);

// components/core/Avatar.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Avatar — square initials chip. Bistro avatars are sharp squares, never round,
 * usually the Bistro-blue identity tile (e.g. "JD" in the diff comment box).
 */
function Avatar({
  name = '',
  size = 32,
  tone = 'primary',
  style,
  ...props
}) {
  const initials = name.split(/[\s_/-]+/).filter(Boolean).slice(0, 2).map(w => w[0]?.toUpperCase()).join('');
  const tones = {
    primary: {
      bg: 'var(--color-primary)',
      fg: 'var(--color-on-primary)'
    },
    neutral: {
      bg: 'var(--color-surface-container-highest)',
      fg: 'var(--color-on-surface-variant)'
    },
    inverse: {
      bg: 'var(--color-inverse-surface)',
      fg: 'var(--color-inverse-on-surface)'
    }
  };
  const t = tones[tone] ?? tones.primary;
  return /*#__PURE__*/React.createElement("span", _extends({
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: size,
      height: size,
      background: t.bg,
      color: t.fg,
      fontFamily: 'var(--font-mono)',
      fontSize: Math.round(size * 0.34),
      fontWeight: 700,
      letterSpacing: '0.02em',
      flexShrink: 0,
      ...style
    }
  }, props), initials || '?');
}
Object.assign(__ds_scope, { Avatar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Avatar.jsx", error: String((e && e.message) || e) }); }

// components/core/Badge.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Badge — a small bordered label. Two register: `outline` (the boxed mono tag,
 * e.g. "GROUP", "RUST", "ACTIVE_DRAFT") and `solid` (filled accent).
 *
 * Status badges (`tone`) tint border + text for review/CI semantics.
 */
function Badge({
  children,
  variant = 'outline',
  tone = 'neutral',
  style,
  ...props
}) {
  const tones = {
    neutral: {
      fg: 'var(--color-on-surface-variant)',
      bd: 'var(--color-outline-variant)',
      bg: 'var(--color-surface-container-low)'
    },
    primary: {
      fg: 'var(--color-primary)',
      bd: 'var(--color-primary)',
      bg: 'var(--color-primary-fixed)'
    },
    success: {
      fg: 'var(--color-success-strong)',
      bd: 'var(--color-success)',
      bg: 'var(--color-diff-add-bg)'
    },
    danger: {
      fg: 'var(--color-error)',
      bd: 'var(--color-error)',
      bg: 'var(--color-error-container)'
    },
    warning: {
      fg: 'var(--color-tertiary)',
      bd: 'var(--color-tertiary-container)',
      bg: 'var(--color-tertiary-fixed)'
    }
  };
  const t = tones[tone] ?? tones.neutral;
  const solid = variant === 'solid';
  return /*#__PURE__*/React.createElement("span", _extends({
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 4,
      padding: '2px 6px',
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      fontWeight: 500,
      letterSpacing: '0.1em',
      textTransform: 'uppercase',
      lineHeight: 1.3,
      whiteSpace: 'nowrap',
      color: solid ? 'var(--color-on-primary)' : t.fg,
      background: solid ? tone === 'neutral' ? 'var(--color-on-surface)' : t.bd : 'transparent',
      border: `1px solid ${solid ? 'transparent' : t.bd}`,
      ...style
    }
  }, props), children);
}
Object.assign(__ds_scope, { Badge });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Badge.jsx", error: String((e && e.message) || e) }); }

// components/core/Button.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Bistro Button — square, technical, keyboard-forward.
 *
 * Zero radius, 1px borders, instant color-only transitions. The default
 * `primary` is Bistro blue; `outline` is the workhorse for secondary actions;
 * `ghost` for tertiary; `danger` for destructive. An optional `keyHint`
 * renders the bound shortcut as a boxed key on the trailing edge.
 */
function Button({
  children,
  variant = 'primary',
  size = 'md',
  keyHint,
  icon,
  disabled = false,
  type = 'button',
  style,
  ...props
}) {
  const sizes = {
    sm: {
      height: 28,
      padding: '0 10px',
      fontSize: 12
    },
    md: {
      height: 36,
      padding: '0 16px',
      fontSize: 13
    },
    lg: {
      height: 44,
      padding: '0 24px',
      fontSize: 14
    }
  };
  const variants = {
    primary: {
      background: 'var(--color-primary)',
      color: 'var(--color-on-primary)',
      border: '1px solid var(--color-primary)'
    },
    outline: {
      background: 'var(--color-surface-container-lowest)',
      color: 'var(--color-on-surface)',
      border: '1px solid var(--color-outline-variant)'
    },
    ghost: {
      background: 'transparent',
      color: 'var(--color-on-surface-variant)',
      border: '1px solid transparent'
    },
    danger: {
      background: 'var(--color-surface-container-lowest)',
      color: 'var(--color-error)',
      border: '1px solid var(--color-error)'
    }
  };
  const sz = sizes[size] ?? sizes.md;
  const vr = variants[variant] ?? variants.primary;
  return /*#__PURE__*/React.createElement("button", _extends({
    type: type,
    disabled: disabled,
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      gap: 8,
      height: sz.height,
      padding: sz.padding,
      fontFamily: 'var(--font-mono)',
      fontSize: sz.fontSize,
      fontWeight: 500,
      letterSpacing: '0.05em',
      textTransform: 'uppercase',
      whiteSpace: 'nowrap',
      cursor: disabled ? 'not-allowed' : 'pointer',
      opacity: disabled ? 0.45 : 1,
      transition: 'background-color 120ms, color 120ms, border-color 120ms',
      ...vr,
      ...style
    },
    onMouseEnter: e => {
      if (disabled) return;
      if (variant === 'primary') e.currentTarget.style.background = '#003ea8';else if (variant === 'outline' || variant === 'ghost') e.currentTarget.style.background = 'var(--color-surface-container)';else if (variant === 'danger') {
        e.currentTarget.style.background = 'var(--color-error)';
        e.currentTarget.style.color = 'var(--color-on-error)';
      }
    },
    onMouseLeave: e => {
      e.currentTarget.style.background = vr.background;
      e.currentTarget.style.color = vr.color;
    }
  }, props), icon && /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: sz.fontSize + 4
    },
    "aria-hidden": "true"
  }, icon), children, keyHint && /*#__PURE__*/React.createElement("kbd", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: sz.fontSize - 2,
      padding: '1px 5px',
      marginLeft: 2,
      border: variant === 'primary' ? '1px solid rgba(255,255,255,0.4)' : '1px solid var(--color-outline-variant)',
      color: 'inherit',
      opacity: 0.85,
      lineHeight: 1.4
    }
  }, keyHint));
}
Object.assign(__ds_scope, { Button });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Button.jsx", error: String((e && e.message) || e) }); }

// components/core/Eyebrow.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Eyebrow — the uppercase monospace label that captions everything in Bistro.
 *
 * Wide tracking, JetBrains Mono. `tone="primary"` is the blue section eyebrow
 * (e.g. "WORKSPACE / GLOBAL"); `tone="muted"` is the gray chrome label.
 */
function Eyebrow({
  children,
  tone = 'muted',
  as = 'span',
  style,
  ...props
}) {
  const Tag = as;
  const colors = {
    muted: 'var(--color-outline)',
    primary: 'var(--color-primary)',
    strong: 'var(--color-on-surface)'
  };
  return /*#__PURE__*/React.createElement(Tag, _extends({
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 12,
      fontWeight: 400,
      letterSpacing: '0.15em',
      textTransform: 'uppercase',
      lineHeight: 1.33,
      color: colors[tone] ?? colors.muted,
      ...style
    }
  }, props), children);
}
Object.assign(__ds_scope, { Eyebrow });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Eyebrow.jsx", error: String((e && e.message) || e) }); }

// components/core/IconButton.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * IconButton — a square, icon-only control built on Material Symbols.
 * Used for toolbar actions, close buttons, row affordances.
 */
function IconButton({
  icon,
  label,
  size = 'md',
  variant = 'ghost',
  disabled = false,
  style,
  ...props
}) {
  const sizes = {
    sm: 28,
    md: 36,
    lg: 44
  };
  const dim = sizes[size] ?? sizes.md;
  const variants = {
    ghost: {
      background: 'transparent',
      color: 'var(--color-on-surface-variant)',
      border: '1px solid transparent'
    },
    outline: {
      background: 'var(--color-surface-container-lowest)',
      color: 'var(--color-on-surface)',
      border: '1px solid var(--color-outline-variant)'
    },
    primary: {
      background: 'var(--color-primary)',
      color: 'var(--color-on-primary)',
      border: '1px solid var(--color-primary)'
    }
  };
  const vr = variants[variant] ?? variants.ghost;
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    "aria-label": label,
    title: label,
    disabled: disabled,
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: dim,
      height: dim,
      cursor: disabled ? 'not-allowed' : 'pointer',
      opacity: disabled ? 0.45 : 1,
      transition: 'background-color 120ms, color 120ms',
      ...vr,
      ...style
    },
    onMouseEnter: e => {
      if (disabled || variant === 'primary') return;
      e.currentTarget.style.background = 'var(--color-surface-container)';
      e.currentTarget.style.color = 'var(--color-on-surface)';
    },
    onMouseLeave: e => {
      e.currentTarget.style.background = vr.background;
      e.currentTarget.style.color = vr.color;
    }
  }, props), /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: Math.round(dim * 0.55)
    },
    "aria-hidden": "true"
  }, icon));
}
Object.assign(__ds_scope, { IconButton });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/IconButton.jsx", error: String((e && e.message) || e) }); }

// components/core/Kbd.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Kbd — a single bound key, rendered as a boxed cap.
 *
 * The atomic unit of Bistro's keyboard-first chrome. Appears in legends,
 * command rows, help overlays, and inline in prose ("press [?]").
 */
function Kbd({
  children,
  tone = 'default',
  style,
  ...props
}) {
  const tones = {
    default: {
      background: 'var(--color-surface-container-lowest)',
      color: 'var(--color-on-surface)',
      border: '1px solid var(--color-outline-variant)'
    },
    muted: {
      background: 'var(--color-surface-container)',
      color: 'var(--color-on-surface-variant)',
      border: '1px solid var(--color-outline-variant)'
    },
    primary: {
      background: 'var(--color-primary)',
      color: 'var(--color-on-primary)',
      border: '1px solid var(--color-primary)'
    }
  };
  return /*#__PURE__*/React.createElement("kbd", _extends({
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      minWidth: 18,
      padding: '2px 6px',
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      lineHeight: 1.2,
      letterSpacing: '0.02em',
      ...(tones[tone] ?? tones.default),
      ...style
    }
  }, props), children);
}
Object.assign(__ds_scope, { Kbd });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/Kbd.jsx", error: String((e && e.message) || e) }); }

// components/core/StatusDot.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
const STATUS = {
  success: {
    color: 'var(--color-success)',
    label: 'CI SUCCESS'
  },
  failure: {
    color: 'var(--color-danger)',
    label: 'CI FAILED'
  },
  running: {
    color: 'var(--color-warning)',
    label: 'CI RUNNING'
  },
  pending: {
    color: 'var(--color-warning)',
    label: 'CI PENDING'
  },
  unknown: {
    color: 'var(--color-neutral-dot)',
    label: 'CI UNKNOWN'
  }
};

/**
 * StatusDot — a hard square status signal. Never a circle.
 *
 * Bistro encodes CI / sync / review state as flat colored squares. `pulse`
 * adds the running/connecting heartbeat.
 */
function StatusDot({
  status = 'unknown',
  size = 12,
  pulse = false,
  title,
  style,
  ...props
}) {
  const s = STATUS[status] ?? STATUS.unknown;
  const animate = pulse || status === 'running' || status === 'pending';
  return /*#__PURE__*/React.createElement("span", _extends({
    role: "img",
    "aria-label": title ?? s.label,
    title: title ?? s.label,
    style: {
      display: 'inline-block',
      width: size,
      height: size,
      background: s.color,
      flexShrink: 0,
      animation: animate ? 'bistro-pulse 1.4s ease-in-out infinite' : undefined,
      ...style
    }
  }, props), /*#__PURE__*/React.createElement("style", null, '@keyframes bistro-pulse{0%,100%{opacity:1}50%{opacity:0.35}}'));
}
Object.assign(__ds_scope, { StatusDot });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/core/StatusDot.jsx", error: String((e && e.message) || e) }); }

// components/feedback/CommandRow.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * CommandRow — one result in the command palette. Leading Material icon,
 * title + mono subtitle, trailing shortcut keys. Selection = left primary
 * accent + highest container fill (cmdk data-selected parity).
 */
function CommandRow({
  icon,
  title,
  subtitle,
  shortcut = [],
  selected = false,
  onClick,
  style,
  ...props
}) {
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    onClick: onClick,
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      width: '100%',
      padding: '12px',
      textAlign: 'left',
      cursor: 'pointer',
      borderLeft: `2px solid ${selected ? 'var(--color-primary)' : 'transparent'}`,
      background: selected ? 'var(--color-surface-container-highest)' : 'transparent',
      transition: 'background-color 120ms, border-color 120ms',
      ...style
    },
    onMouseEnter: e => {
      if (!selected) e.currentTarget.style.background = 'var(--color-surface-container-highest)';
    },
    onMouseLeave: e => {
      if (!selected) e.currentTarget.style.background = 'transparent';
    }
  }, props), /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      minWidth: 0
    }
  }, icon && /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 20,
      color: selected ? 'var(--color-primary)' : 'var(--color-on-surface-variant)',
      flexShrink: 0
    },
    "aria-hidden": "true"
  }, icon), /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      minWidth: 0
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      fontWeight: 500,
      color: 'var(--color-on-surface)',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap'
    }
  }, title), subtitle && /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      color: 'var(--color-outline)',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap'
    }
  }, subtitle))), shortcut.length > 0 && /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 4,
      flexShrink: 0,
      marginLeft: 16,
      opacity: selected ? 1 : 0.5
    }
  }, shortcut.map(k => /*#__PURE__*/React.createElement("kbd", {
    key: k,
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      padding: '2px 6px',
      background: 'var(--color-surface)',
      border: '1px solid var(--color-outline-variant)',
      color: 'var(--color-on-surface-variant)'
    }
  }, k))));
}
Object.assign(__ds_scope, { CommandRow });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/CommandRow.jsx", error: String((e && e.message) || e) }); }

// components/feedback/LineFocusChip.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * LineFocusChip — the small dark inverse chip from the diff view that reports
 * cursor position ("LINE_FOCUS: 44" + path). Also serves as Bistro's tooltip:
 * dark inverse surface, mono, square.
 */
function LineFocusChip({
  label,
  sub,
  style,
  ...props
}) {
  return /*#__PURE__*/React.createElement("span", _extends({
    style: {
      display: 'inline-flex',
      flexDirection: 'column',
      alignItems: 'flex-start',
      ...style
    }
  }, props), /*#__PURE__*/React.createElement("span", {
    style: {
      padding: '4px 10px',
      background: 'var(--color-inverse-surface)',
      color: 'var(--color-inverse-on-surface)',
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      letterSpacing: '0.12em',
      textTransform: 'uppercase'
    }
  }, label), sub && /*#__PURE__*/React.createElement("span", {
    style: {
      padding: '3px 10px',
      background: 'var(--color-surface-container-high)',
      color: 'var(--color-outline)',
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      letterSpacing: '0.1em',
      textTransform: 'uppercase',
      borderTop: '1px solid var(--color-outline-variant)'
    }
  }, sub));
}
Object.assign(__ds_scope, { LineFocusChip });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/LineFocusChip.jsx", error: String((e && e.message) || e) }); }

// components/feedback/Toast.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Toast — terse action feedback. A flat bar: status square, mono message,
 * optional key hint. Bistro never celebrates; it confirms. ("PR approved.")
 */
function Toast({
  message,
  tone = 'success',
  keyHint,
  style,
  ...props
}) {
  const dot = {
    success: 'var(--color-success)',
    error: 'var(--color-danger)',
    info: 'var(--color-primary)',
    warning: 'var(--color-warning)'
  }[tone] ?? 'var(--color-success)';
  return /*#__PURE__*/React.createElement("div", _extends({
    role: "status",
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 12,
      padding: '10px 14px',
      background: 'var(--color-inverse-surface)',
      color: 'var(--color-inverse-on-surface)',
      boxShadow: 'var(--shadow-ambient)',
      ...style
    }
  }, props), /*#__PURE__*/React.createElement("span", {
    style: {
      width: 8,
      height: 8,
      background: dot,
      flexShrink: 0
    },
    "aria-hidden": "true"
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 12,
      letterSpacing: '0.04em'
    }
  }, message), keyHint && /*#__PURE__*/React.createElement("kbd", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      padding: '2px 6px',
      border: '1px solid rgba(255,255,255,0.25)',
      color: 'inherit',
      opacity: 0.8
    }
  }, keyHint));
}
Object.assign(__ds_scope, { Toast });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/feedback/Toast.jsx", error: String((e && e.message) || e) }); }

// components/forms/Checkbox.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Checkbox — square, of course. Checked state fills Bistro blue with a
 * Material check glyph. Includes an optional inline label.
 */
function Checkbox({
  checked = false,
  onChange,
  label,
  disabled = false,
  id,
  style,
  ...props
}) {
  const reactId = React.useId();
  const cid = id ?? reactId;
  return /*#__PURE__*/React.createElement("label", {
    htmlFor: cid,
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 8,
      cursor: disabled ? 'not-allowed' : 'pointer',
      opacity: disabled ? 0.5 : 1,
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      color: 'var(--color-on-surface)',
      ...style
    }
  }, /*#__PURE__*/React.createElement("input", _extends({
    id: cid,
    type: "checkbox",
    checked: checked,
    disabled: disabled,
    onChange: onChange,
    style: {
      position: 'absolute',
      opacity: 0,
      width: 0,
      height: 0
    }
  }, props)), /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: 18,
      height: 18,
      flexShrink: 0,
      background: checked ? 'var(--color-primary)' : 'var(--color-surface-container-lowest)',
      border: `1px solid ${checked ? 'var(--color-primary)' : 'var(--color-outline-variant)'}`,
      color: 'var(--color-on-primary)',
      transition: 'background-color 120ms, border-color 120ms'
    }
  }, checked && /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 16,
      fontVariationSettings: '"wght" 600'
    },
    "aria-hidden": "true"
  }, "check")), label && /*#__PURE__*/React.createElement("span", null, label));
}
Object.assign(__ds_scope, { Checkbox });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Checkbox.jsx", error: String((e && e.message) || e) }); }

// components/forms/Input.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Input — square text field. Flat fill, no border at rest; a 1px primary
 * underline-free ring on focus. Optional leading Material Symbol.
 */
function Input({
  icon,
  invalid = false,
  style,
  ...props
}) {
  const [focused, setFocused] = React.useState(false);
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      height: 36,
      padding: '0 12px',
      background: 'var(--color-surface-container-low)',
      border: `1px solid ${invalid ? 'var(--color-error)' : focused ? 'var(--color-primary)' : 'transparent'}`,
      boxShadow: focused ? 'inset 0 0 0 1px var(--color-primary)' : 'none',
      transition: 'border-color 120ms, box-shadow 120ms'
    }
  }, icon && /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 18,
      color: 'var(--color-outline)'
    },
    "aria-hidden": "true"
  }, icon), /*#__PURE__*/React.createElement("input", _extends({
    onFocus: e => {
      setFocused(true);
      props.onFocus?.(e);
    },
    onBlur: e => {
      setFocused(false);
      props.onBlur?.(e);
    },
    style: {
      flex: 1,
      minWidth: 0,
      height: '100%',
      border: 'none',
      outline: 'none',
      background: 'transparent',
      color: 'var(--color-on-surface)',
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      ...style
    }
  }, props)));
}
Object.assign(__ds_scope, { Input });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Input.jsx", error: String((e && e.message) || e) }); }

// components/forms/SegmentedControl.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * SegmentedControl — the boxed filter row from the inbox ("Open / Draft / All /
 * Needs Action"). Each option carries an optional count. Active = blue fill.
 */
function SegmentedControl({
  options = [],
  value,
  onChange,
  style,
  ...props
}) {
  return /*#__PURE__*/React.createElement("div", _extends({
    role: "tablist",
    style: {
      display: 'inline-flex',
      border: '1px solid var(--color-outline-variant)',
      ...style
    }
  }, props), options.map((opt, i) => {
    const val = typeof opt === 'string' ? opt : opt.value;
    const labelTxt = typeof opt === 'string' ? opt : opt.label;
    const count = typeof opt === 'string' ? undefined : opt.count;
    const active = val === value;
    return /*#__PURE__*/React.createElement("button", {
      key: val,
      type: "button",
      role: "tab",
      "aria-selected": active,
      onClick: () => onChange?.(val),
      style: {
        display: 'inline-flex',
        alignItems: 'center',
        gap: 6,
        padding: '6px 12px',
        fontFamily: 'var(--font-mono)',
        fontSize: 11,
        letterSpacing: '0.08em',
        textTransform: 'uppercase',
        whiteSpace: 'nowrap',
        borderLeft: i === 0 ? 'none' : '1px solid var(--color-outline-variant)',
        background: active ? 'var(--color-primary)' : 'transparent',
        color: active ? 'var(--color-on-primary)' : 'var(--color-on-surface-variant)',
        transition: 'background-color 120ms, color 120ms'
      },
      onMouseEnter: e => {
        if (!active) e.currentTarget.style.background = 'var(--color-surface-container)';
      },
      onMouseLeave: e => {
        if (!active) e.currentTarget.style.background = 'transparent';
      }
    }, labelTxt, count !== undefined && /*#__PURE__*/React.createElement("span", {
      style: {
        opacity: 0.7,
        fontSize: 10
      }
    }, count));
  }));
}
Object.assign(__ds_scope, { SegmentedControl });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/SegmentedControl.jsx", error: String((e && e.message) || e) }); }

// components/forms/Switch.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Switch — square track + square thumb. No pills in Bistro. Settings toggles.
 */
function Switch({
  checked = false,
  onChange,
  label,
  disabled = false,
  id,
  style,
  ...props
}) {
  const reactId = React.useId();
  const cid = id ?? reactId;
  return /*#__PURE__*/React.createElement("label", {
    htmlFor: cid,
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 10,
      cursor: disabled ? 'not-allowed' : 'pointer',
      opacity: disabled ? 0.5 : 1,
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      color: 'var(--color-on-surface)',
      ...style
    }
  }, /*#__PURE__*/React.createElement("input", _extends({
    id: cid,
    type: "checkbox",
    role: "switch",
    checked: checked,
    disabled: disabled,
    onChange: onChange,
    style: {
      position: 'absolute',
      opacity: 0,
      width: 0,
      height: 0
    }
  }, props)), /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'relative',
      width: 36,
      height: 18,
      flexShrink: 0,
      background: checked ? 'var(--color-primary)' : 'var(--color-surface-container-highest)',
      border: `1px solid ${checked ? 'var(--color-primary)' : 'var(--color-outline-variant)'}`,
      transition: 'background-color 140ms'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      position: 'absolute',
      top: 1,
      left: checked ? 19 : 1,
      width: 14,
      height: 14,
      background: checked ? 'var(--color-on-primary)' : 'var(--color-outline)',
      transition: 'left 140ms cubic-bezier(0.4,0,0.2,1)'
    }
  })), label && /*#__PURE__*/React.createElement("span", null, label));
}
Object.assign(__ds_scope, { Switch });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/forms/Switch.jsx", error: String((e && e.message) || e) }); }

// components/layout/Panel.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Panel — the white container plane. A flat surface with a 1px hairline border;
 * no shadow, no radius. Optional `title` (mono eyebrow) header bar.
 */
function Panel({
  children,
  title,
  action,
  padded = true,
  elevated = false,
  style,
  ...props
}) {
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      background: 'var(--color-surface-container-lowest)',
      border: '1px solid var(--color-outline-variant)',
      boxShadow: elevated ? 'var(--shadow-ambient)' : 'none',
      ...style
    }
  }, props), title && /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      padding: '10px 16px',
      borderBottom: '1px solid var(--color-outline-variant)'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 12,
      letterSpacing: '0.15em',
      textTransform: 'uppercase',
      color: 'var(--color-outline)'
    }
  }, title), action), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: padded ? 16 : 0
    }
  }, children));
}
Object.assign(__ds_scope, { Panel });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/layout/Panel.jsx", error: String((e && e.message) || e) }); }

// components/layout/SystemBar.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * SystemBar — the fixed bottom chrome strip. Three regions: left legend
 * (key/desc pairs), center slot, right status. Mono, uppercase, 32px tall.
 * The "SYSTEM_STATUS: OPERATIONAL // V.01" footer.
 */
function SystemBar({
  legend = [],
  center,
  status,
  version,
  statusTone = 'success',
  style,
  ...props
}) {
  const dot = {
    success: 'var(--color-success)',
    warning: 'var(--color-warning)',
    danger: 'var(--color-danger)',
    idle: 'var(--color-neutral-dot)'
  }[statusTone] ?? 'var(--color-success)';
  return /*#__PURE__*/React.createElement("footer", _extends({
    style: {
      display: 'flex',
      alignItems: 'center',
      height: 32,
      flexShrink: 0,
      borderTop: '1px solid var(--color-outline-variant)',
      background: 'var(--color-surface-container-high)',
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      letterSpacing: '0.12em',
      textTransform: 'uppercase',
      ...style
    }
  }, props), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      display: 'flex',
      alignItems: 'center',
      gap: 16,
      padding: '0 16px',
      overflow: 'hidden'
    }
  }, legend.map(item => /*#__PURE__*/React.createElement("span", {
    key: item.key,
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 8,
      whiteSpace: 'nowrap'
    }
  }, /*#__PURE__*/React.createElement("kbd", {
    style: {
      background: 'var(--color-surface-container-lowest)',
      border: '1px solid var(--color-outline-variant)',
      padding: '2px 6px',
      fontSize: 9,
      color: 'var(--color-on-surface)'
    }
  }, item.key), /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--color-outline)'
    }
  }, item.label)))), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      overflow: 'hidden'
    }
  }, center), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      display: 'flex',
      justifyContent: 'flex-end',
      alignItems: 'center',
      gap: 10,
      padding: '0 16px'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--color-primary)',
      fontWeight: 500
    }
  }, status ?? 'System_Status: Operational'), version && /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--color-primary)'
    }
  }, "// ", version), /*#__PURE__*/React.createElement("span", {
    style: {
      width: 8,
      height: 8,
      background: dot
    },
    "aria-hidden": "true"
  })));
}
Object.assign(__ds_scope, { SystemBar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/layout/SystemBar.jsx", error: String((e && e.message) || e) }); }

// components/layout/Toolbar.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Toolbar — a horizontal command strip with a hairline underline. Holds the
 * breadcrumb / title on the left and actions on the right. The diff-view and
 * inbox top bars are Toolbars.
 */
function Toolbar({
  children,
  leading,
  trailing,
  style,
  ...props
}) {
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      gap: 16,
      minHeight: 56,
      padding: '0 20px',
      borderBottom: '1px solid var(--color-outline-variant)',
      background: 'var(--color-surface-container-lowest)',
      ...style
    }
  }, props), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      minWidth: 0
    }
  }, leading, children), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      flexShrink: 0
    }
  }, trailing));
}
Object.assign(__ds_scope, { Toolbar });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/layout/Toolbar.jsx", error: String((e && e.message) || e) }); }

// components/review/CommentCard.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * CommentCard — an inline review comment. Square avatar, mono author handle,
 * a status tag (e.g. ACTIVE_DRAFT), body, and optional action row. Matches the
 * diff-view comment composer / thread entry.
 */
function CommentCard({
  author,
  tag,
  time,
  children,
  actions,
  style,
  ...props
}) {
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      background: 'var(--color-surface-container-lowest)',
      border: '1px solid var(--color-outline-variant)',
      ...style
    }
  }, props), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      padding: '12px 16px',
      borderBottom: '1px solid var(--color-outline-variant)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 10,
      minWidth: 0
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      width: 28,
      height: 28,
      background: 'var(--color-primary)',
      color: 'var(--color-on-primary)',
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      fontWeight: 700,
      flexShrink: 0
    }
  }, (author ?? '?').split(/[\s_/-]+/).filter(Boolean).slice(0, 2).map(w => w[0]?.toUpperCase()).join('')), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 13,
      fontWeight: 700,
      letterSpacing: '0.04em',
      color: 'var(--color-on-surface)',
      textTransform: 'uppercase'
    }
  }, author)), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12
    }
  }, time && /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      color: 'var(--color-outline-variant)'
    }
  }, time), tag && /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      letterSpacing: '0.12em',
      textTransform: 'uppercase',
      color: 'var(--color-outline)'
    }
  }, tag))), /*#__PURE__*/React.createElement("div", {
    style: {
      padding: 16,
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      lineHeight: 1.5,
      color: children ? 'var(--color-on-surface)' : 'var(--color-outline)'
    }
  }, children ?? 'Type_comment_here…'), actions && /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'flex-end',
      gap: 8,
      padding: '12px 16px',
      borderTop: '1px solid var(--color-outline-variant)'
    }
  }, actions));
}
Object.assign(__ds_scope, { CommentCard });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/review/CommentCard.jsx", error: String((e && e.message) || e) }); }

// components/review/DiffLine.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * DiffLine — one row of a unified diff. Old/new line-number gutters, a +/-/·
 * marker, and monospace code. `type` tints the row: add (green), remove (red),
 * context (plain), focus (primary-outlined active line).
 */
function DiffLine({
  oldNo,
  newNo,
  type = 'context',
  children,
  focused = false,
  style,
  ...props
}) {
  const bg = {
    add: 'var(--color-diff-add-bg)',
    remove: 'var(--color-diff-remove-bg)',
    context: 'transparent'
  }[type] ?? 'transparent';
  const marker = {
    add: '+',
    remove: '-',
    context: ''
  }[type] ?? '';
  const codeColor = {
    add: 'var(--color-on-surface)',
    remove: 'var(--color-on-surface)',
    context: 'var(--color-on-surface)'
  }[type];
  return /*#__PURE__*/React.createElement("div", _extends({
    style: {
      display: 'grid',
      gridTemplateColumns: '52px 52px 1fr',
      alignItems: 'stretch',
      background: bg,
      boxShadow: focused ? 'inset 0 0 0 1px var(--color-primary)' : 'none',
      fontFamily: 'var(--font-mono)',
      fontSize: 13,
      lineHeight: '24px',
      ...style
    }
  }, props), /*#__PURE__*/React.createElement("span", {
    style: {
      textAlign: 'center',
      color: 'var(--color-outline-variant)',
      userSelect: 'none',
      background: type === 'remove' ? 'var(--color-diff-remove-gutter)' : type === 'add' ? 'transparent' : 'transparent'
    }
  }, oldNo ?? (type === 'add' ? '' : '')), /*#__PURE__*/React.createElement("span", {
    style: {
      textAlign: 'center',
      color: 'var(--color-outline-variant)',
      userSelect: 'none',
      background: type === 'add' ? 'var(--color-diff-add-gutter)' : 'transparent'
    }
  }, newNo ?? (type === 'remove' ? '' : '')), /*#__PURE__*/React.createElement("code", {
    style: {
      paddingLeft: 12,
      paddingRight: 12,
      color: codeColor,
      whiteSpace: 'pre',
      overflow: 'hidden'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'inline-block',
      width: 14,
      color: type === 'add' ? 'var(--color-diff-add-text)' : type === 'remove' ? 'var(--color-diff-remove-text)' : 'transparent'
    }
  }, marker), children));
}
Object.assign(__ds_scope, { DiffLine });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/review/DiffLine.jsx", error: String((e && e.message) || e) }); }

// components/review/PRRow.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
function relTime(label) {
  return label;
}

/**
 * PRRow — one pull request in the inbox. 5-region grid: CI square · #number ·
 * title · @author · relative time. Selection paints a left primary accent +
 * tinted fill. Crosshair cursor — the technical signature.
 */
function PRRow({
  number,
  title,
  author,
  time,
  ci = 'unknown',
  selected = false,
  onClick,
  style,
  ...props
}) {
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    onClick: onClick,
    style: {
      display: 'grid',
      gridTemplateColumns: '24px 60px 1fr 120px 100px',
      alignItems: 'center',
      gap: 16,
      width: '100%',
      padding: '12px 16px',
      textAlign: 'left',
      cursor: 'crosshair',
      background: selected ? 'var(--color-surface-container-low)' : 'transparent',
      borderLeft: `2px solid ${selected ? 'var(--color-primary)' : 'transparent'}`,
      transition: 'background-color 120ms, border-color 120ms',
      ...style
    },
    onMouseEnter: e => {
      if (selected) return;
      e.currentTarget.style.background = 'var(--color-surface-container-low)';
      e.currentTarget.style.borderLeftColor = 'var(--color-primary)';
    },
    onMouseLeave: e => {
      if (selected) return;
      e.currentTarget.style.background = 'transparent';
      e.currentTarget.style.borderLeftColor = 'transparent';
    }
  }, props), /*#__PURE__*/React.createElement("span", {
    style: {
      width: 12,
      height: 12,
      background: {
        success: 'var(--color-success)',
        failure: 'var(--color-danger)',
        running: 'var(--color-warning)',
        pending: 'var(--color-warning)',
        unknown: 'var(--color-neutral-dot)'
      }[ci] ?? 'var(--color-neutral-dot)'
    },
    "aria-hidden": "true"
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      color: 'var(--color-outline)'
    }
  }, "#", number), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      fontWeight: 500,
      color: 'var(--color-on-surface)',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap'
    }
  }, title), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      color: 'var(--color-outline-variant)',
      textAlign: 'right',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  }, "@", author), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      textAlign: 'right',
      color: selected ? 'var(--color-primary)' : 'var(--color-outline-variant)'
    }
  }, relTime(time)));
}
Object.assign(__ds_scope, { PRRow });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/review/PRRow.jsx", error: String((e && e.message) || e) }); }

// ui_kits/bistro-app/DiffScreen.jsx
try { (() => {
// Bistro UI kit — Diff View. Mirrors Diff-View (Minimal Light).
const {
  Eyebrow: DiffEyebrow,
  Badge: DiffBadge,
  DiffLine,
  CommentCard: DiffComment,
  Button: DiffButton,
  LineFocusChip
} = window.BistroDesignSystem_8e22c3;
function DiffScreen({
  onApprove
}) {
  const d = window.BistroData.diff;
  const [composing, setComposing] = React.useState(true);
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: '100%',
      display: 'flex',
      background: 'var(--color-surface)',
      position: 'relative'
    }
  }, /*#__PURE__*/React.createElement("aside", {
    style: {
      width: 268,
      flexShrink: 0,
      borderRight: '1px solid var(--color-outline-variant)',
      padding: '24px 16px',
      background: 'var(--color-surface)'
    }
  }, /*#__PURE__*/React.createElement(DiffEyebrow, {
    style: {
      fontSize: 11,
      display: 'block',
      marginBottom: 16
    }
  }, "Files_changed (", window.BistroData.pr.files, ")"), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 2
    }
  }, d.files.map(f => /*#__PURE__*/React.createElement("div", {
    key: f.path,
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      padding: '8px 10px',
      cursor: 'crosshair',
      background: f.active ? 'var(--color-surface-container-lowest)' : 'transparent',
      border: f.active ? '1px solid var(--color-primary)' : '1px solid transparent'
    }
  }, /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 16,
      color: f.active ? 'var(--color-primary)' : 'var(--color-outline)'
    }
  }, "description"), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 12,
      color: f.active ? 'var(--color-primary)' : 'var(--color-on-surface-variant)',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap'
    }
  }, f.path))))), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      minWidth: 0,
      overflowY: 'auto'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      padding: '20px 28px',
      borderBottom: '1px solid var(--color-outline-variant)'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 15,
      color: 'var(--color-on-surface)'
    }
  }, d.file), /*#__PURE__*/React.createElement(DiffBadge, null, d.lang), /*#__PURE__*/React.createElement("span", {
    style: {
      marginLeft: 'auto',
      fontFamily: 'var(--font-mono)',
      fontSize: 15,
      color: 'var(--color-diff-add-text)'
    }
  }, "+", d.added), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 15,
      color: 'var(--color-diff-remove-text)'
    }
  }, "-", d.removed)), /*#__PURE__*/React.createElement("div", null, d.lines.slice(0, 5).map((ln, i) => /*#__PURE__*/React.createElement(DiffLine, {
    key: i,
    oldNo: ln.o,
    newNo: ln.n,
    type: ln.t,
    focused: ln.focused
  }, ln.c)), composing && /*#__PURE__*/React.createElement("div", {
    style: {
      padding: '24px 56px 24px 104px',
      background: 'var(--color-surface-container-low)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      maxWidth: 1100
    }
  }, /*#__PURE__*/React.createElement(DiffComment, {
    author: "J_Doe System Arch",
    tag: "Active_draft",
    actions: /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement(DiffButton, {
      variant: "ghost",
      size: "sm",
      onClick: () => setComposing(false)
    }, "Cancel"), /*#__PURE__*/React.createElement(DiffButton, {
      variant: "primary",
      size: "sm",
      onClick: () => setComposing(false)
    }, "Comment"))
  }))), d.lines.slice(5).map((ln, i) => /*#__PURE__*/React.createElement(DiffLine, {
    key: i,
    oldNo: ln.o,
    newNo: ln.n,
    type: ln.t,
    focused: ln.focused
  }, ln.c)))), /*#__PURE__*/React.createElement("div", {
    style: {
      position: 'absolute',
      right: 24,
      bottom: 24
    }
  }, /*#__PURE__*/React.createElement(LineFocusChip, {
    label: "Line_focus: 44",
    sub: d.file
  })));
}
window.DiffScreen = DiffScreen;
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/bistro-app/DiffScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/bistro-app/InboxScreen.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
// Bistro UI kit — PR Inbox. Mirrors PR-Inbox (Minimal Light).
const {
  Eyebrow: InboxEyebrow,
  Badge: InboxBadge,
  PRRow,
  SegmentedControl
} = window.BistroDesignSystem_8e22c3;
function RepoGroup({
  group,
  baseIndex,
  selected,
  onSelect,
  onOpen
}) {
  const [open, setOpen] = React.useState(!group.collapsed);
  return /*#__PURE__*/React.createElement("div", {
    style: {
      marginBottom: open ? 4 : 0
    }
  }, /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: () => setOpen(o => !o),
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      width: '100%',
      padding: '14px 12px',
      background: 'var(--color-surface-container-low)',
      cursor: 'pointer',
      border: 'none',
      borderLeft: '2px solid transparent'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 16
    }
  }, /*#__PURE__*/React.createElement(InboxBadge, {
    tone: group.accent ? 'primary' : 'neutral'
  }, "Group"), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      fontWeight: 700,
      letterSpacing: '-0.01em',
      color: group.collapsed && !open ? 'var(--color-outline)' : 'var(--color-on-surface)'
    }
  }, group.key)), /*#__PURE__*/React.createElement(InboxEyebrow, {
    style: {
      fontSize: 10
    }
  }, open ? 'Collapse' : 'Expand', " [Space]")), open && /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 2
    }
  }, group.prs.map((pr, i) => /*#__PURE__*/React.createElement(PRRow, _extends({
    key: pr.number
  }, pr, {
    selected: selected === baseIndex + i,
    onClick: () => {
      onSelect(baseIndex + i);
      onOpen(pr);
    }
  })))));
}
function InboxScreen({
  onOpenPR
}) {
  const data = window.BistroData;
  const [filter, setFilter] = React.useState('open');
  const [selected, setSelected] = React.useState(0);
  let cursor = 0;
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: '100%',
      overflowY: 'auto',
      background: 'var(--color-surface)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      maxWidth: 1600,
      margin: '0 auto',
      padding: '40px 48px 64px'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'flex-start',
      justifyContent: 'space-between',
      marginBottom: 4
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 18
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--color-outline)'
    }
  }, "bistro-ui"), /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--color-outline-variant)',
      margin: '0 8px'
    }
  }, "/"), /*#__PURE__*/React.createElement("span", {
    style: {
      color: 'var(--color-on-surface)',
      fontWeight: 700
    }
  }, "web-client")), /*#__PURE__*/React.createElement("div", {
    style: {
      border: '1px solid var(--color-outline-variant)',
      padding: '10px 16px',
      background: 'var(--color-surface-container-lowest)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 8
    }
  }, /*#__PURE__*/React.createElement(InboxEyebrow, {
    style: {
      fontSize: 11
    }
  }, "Filter:"), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      fontWeight: 700,
      color: 'var(--color-on-surface)',
      letterSpacing: '0.06em'
    }
  }, "IS:OPEN AUTHOR:ME")), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 8,
      marginTop: 6
    }
  }, /*#__PURE__*/React.createElement(InboxEyebrow, {
    style: {
      fontSize: 11
    }
  }, "Sort:"), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      color: 'var(--color-on-surface-variant)',
      letterSpacing: '0.06em'
    }
  }, "NEWEST_FIRST")))), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      borderBottom: '1px solid var(--color-outline-variant)',
      paddingBottom: 14,
      marginBottom: 8
    }
  }, /*#__PURE__*/React.createElement("h1", {
    style: {
      margin: 0,
      fontFamily: 'var(--font-mono)',
      fontSize: 22,
      letterSpacing: '0.18em',
      textTransform: 'uppercase',
      color: 'var(--color-outline)',
      fontWeight: 400
    }
  }, "Open Pull Requests"), /*#__PURE__*/React.createElement("span", {
    style: {
      background: 'var(--color-primary)',
      color: 'var(--color-on-primary)',
      fontFamily: 'var(--font-mono)',
      fontSize: 12,
      padding: '8px 14px',
      letterSpacing: '0.08em'
    }
  }, "CMD + K")), /*#__PURE__*/React.createElement("div", {
    style: {
      marginBottom: 20
    }
  }, /*#__PURE__*/React.createElement(SegmentedControl, {
    value: filter,
    onChange: setFilter,
    options: [{
      value: 'needs',
      label: 'Needs Action',
      count: 2
    }, {
      value: 'open',
      label: 'Open',
      count: 6
    }, {
      value: 'draft',
      label: 'Draft',
      count: 1
    }, {
      value: 'all',
      label: 'All'
    }]
  })), data.repos.map(g => {
    const base = cursor;
    cursor += g.prs.length;
    return /*#__PURE__*/React.createElement(RepoGroup, {
      key: g.key,
      group: g,
      baseIndex: base,
      selected: selected,
      onSelect: setSelected,
      onOpen: onOpenPR
    });
  })));
}
window.InboxScreen = InboxScreen;
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/bistro-app/InboxScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/bistro-app/LoginScreen.jsx
try { (() => {
// Bistro UI kit — Login screen. Mirrors Bistro-Login (Minimal).
const {
  Eyebrow
} = window.BistroDesignSystem_8e22c3;
function ProviderRow({
  icon,
  label,
  keyHint,
  onClick
}) {
  const [hover, setHover] = React.useState(false);
  return /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: onClick,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      width: '100%',
      padding: '0 20px',
      height: 64,
      cursor: 'pointer',
      background: hover ? 'var(--color-surface-container-low)' : 'var(--color-surface-container-lowest)',
      border: `1px solid ${hover ? 'var(--color-primary)' : 'var(--color-outline-variant)'}`,
      transition: 'background-color 120ms, border-color 120ms'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 16
    }
  }, /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 26,
      color: 'var(--color-on-surface)'
    }
  }, icon), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 18,
      fontWeight: 700,
      color: 'var(--color-on-surface)'
    }
  }, label)), /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 10
    }
  }, /*#__PURE__*/React.createElement("kbd", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 11,
      padding: '3px 9px',
      border: '1px solid var(--color-outline-variant)',
      color: 'var(--color-on-surface-variant)'
    }
  }, keyHint), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      letterSpacing: '0.12em',
      textTransform: 'uppercase',
      color: 'var(--color-outline)'
    }
  }, "Key hint")));
}
function LoginScreen({
  onLogin
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: '100%',
      display: 'flex',
      flexDirection: 'column',
      background: 'var(--color-surface)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      width: 588,
      background: 'var(--color-surface-container-lowest)',
      boxShadow: 'var(--shadow-ambient)',
      padding: 48
    }
  }, /*#__PURE__*/React.createElement("h1", {
    style: {
      margin: 0,
      fontFamily: 'var(--font-sans)',
      fontSize: 52,
      fontWeight: 800,
      letterSpacing: '-0.04em',
      textTransform: 'uppercase',
      color: 'var(--color-on-surface)'
    }
  }, "Bistro"), /*#__PURE__*/React.createElement("p", {
    style: {
      margin: '16px 0 40px',
      fontFamily: 'var(--font-sans)',
      fontSize: 16,
      color: 'var(--color-on-surface-variant)'
    }
  }, "Connect your platform to begin reviewing code"), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 16
    }
  }, /*#__PURE__*/React.createElement(ProviderRow, {
    icon: "terminal",
    label: "GitHub",
    keyHint: "G",
    onClick: onLogin
  }), /*#__PURE__*/React.createElement(ProviderRow, {
    icon: "folder_open",
    label: "GitLab",
    keyHint: "L",
    onClick: onLogin
  }), /*#__PURE__*/React.createElement(ProviderRow, {
    icon: "local_cafe",
    label: "Gitea",
    keyHint: "T",
    onClick: onLogin
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 56,
      display: 'flex',
      alignItems: 'flex-end',
      justifyContent: 'space-between'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 4
    }
  }, /*#__PURE__*/React.createElement(Eyebrow, {
    style: {
      fontSize: 10,
      color: 'var(--color-outline-variant)'
    }
  }, "Status:"), /*#__PURE__*/React.createElement(Eyebrow, {
    style: {
      fontSize: 10,
      color: 'var(--color-outline-variant)'
    }
  }, "Ready"), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 10,
      color: 'var(--color-outline-variant)',
      textTransform: 'none'
    }
  }, "v4.0.2-stable")), /*#__PURE__*/React.createElement("div", {
    style: {
      width: 96,
      height: 1,
      background: 'var(--color-outline-variant)'
    }
  })))), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      height: 40,
      padding: '0 24px',
      borderTop: '1px solid var(--color-outline-variant)',
      background: 'var(--color-surface-container-high)'
    }
  }, /*#__PURE__*/React.createElement(Eyebrow, {
    style: {
      fontSize: 11
    }
  }, "Bistro // Technical Atelier\xA0\xA0\xA9 2026"), /*#__PURE__*/React.createElement("div", {
    style: {
      marginLeft: 'auto',
      display: 'flex',
      gap: 18
    }
  }, /*#__PURE__*/React.createElement(Eyebrow, {
    style: {
      fontSize: 11,
      color: 'var(--color-outline-variant)'
    }
  }, "Documentation"), /*#__PURE__*/React.createElement(Eyebrow, {
    style: {
      fontSize: 11,
      color: 'var(--color-outline-variant)'
    }
  }, "Support"), /*#__PURE__*/React.createElement(Eyebrow, {
    style: {
      fontSize: 11,
      color: 'var(--color-outline-variant)'
    }
  }, "Status"))));
}
window.LoginScreen = LoginScreen;
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/bistro-app/LoginScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/bistro-app/Overlays.jsx
try { (() => {
// Bistro UI kit — Command Palette + Help overlays.
const {
  CommandRow,
  Kbd: OvlKbd,
  Eyebrow: OvlEyebrow
} = window.BistroDesignSystem_8e22c3;
function CommandPalette({
  onClose,
  onCommand
}) {
  const data = window.BistroData;
  const [query, setQuery] = React.useState('');
  const [sel, setSel] = React.useState(0);
  const inputRef = React.useRef(null);
  React.useEffect(() => {
    inputRef.current?.focus();
  }, []);
  const recents = data.repos[0].prs.slice(0, 3);
  const filteredCmds = data.commands.filter(c => c.title.toLowerCase().includes(query.toLowerCase()));
  const items = [...recents.map(p => ({
    kind: 'pr',
    p
  })), ...filteredCmds.map(c => ({
    kind: 'cmd',
    c
  }))];
  const onKey = e => {
    if (e.key === 'Escape') {
      onClose();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSel(s => Math.min(s + 1, items.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSel(s => Math.max(s - 1, 0));
    } else if (e.key === 'Enter') {
      const it = items[sel];
      if (it?.kind === 'cmd') onCommand(it.c.id);else onClose();
    }
  };
  return /*#__PURE__*/React.createElement("div", {
    onMouseDown: e => {
      if (e.target === e.currentTarget) onClose();
    },
    onKeyDown: onKey,
    style: {
      position: 'absolute',
      inset: 0,
      zIndex: 40,
      backdropFilter: 'blur(20px)',
      WebkitBackdropFilter: 'blur(20px)',
      background: 'rgba(248,249,250,0.4)',
      display: 'flex',
      justifyContent: 'center',
      paddingTop: 120
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      width: 680,
      maxHeight: 560,
      background: 'var(--color-surface-container-lowest)',
      border: '1px solid var(--color-primary)',
      boxShadow: 'var(--shadow-ambient)',
      display: 'flex',
      flexDirection: 'column'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      padding: '0 16px',
      height: 56,
      borderBottom: '1px solid var(--color-outline-variant)'
    }
  }, /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      color: 'var(--color-primary)'
    }
  }, "search"), /*#__PURE__*/React.createElement("input", {
    ref: inputRef,
    value: query,
    onChange: e => {
      setQuery(e.target.value);
      setSel(0);
    },
    placeholder: "Type a command or search\u2026",
    style: {
      flex: 1,
      border: 'none',
      outline: 'none',
      background: 'transparent',
      fontFamily: 'var(--font-sans)',
      fontSize: 18,
      color: 'var(--color-on-surface)'
    }
  }), /*#__PURE__*/React.createElement(OvlKbd, {
    tone: "muted"
  }, "ESC"), /*#__PURE__*/React.createElement(OvlEyebrow, {
    style: {
      fontSize: 10
    }
  }, "to close")), /*#__PURE__*/React.createElement("div", {
    style: {
      overflowY: 'auto',
      padding: '8px 8px'
    }
  }, recents.length > 0 && /*#__PURE__*/React.createElement(OvlEyebrow, {
    style: {
      fontSize: 10,
      display: 'block',
      padding: '8px 10px'
    }
  }, "Recently opened PRs"), recents.map((p, i) => /*#__PURE__*/React.createElement(CommandRow, {
    key: 'pr' + p.number,
    icon: "rebase",
    title: p.title,
    subtitle: `web-client#${p.number} • ${p.time} by @${p.author}`,
    selected: sel === i,
    onClick: onClose
  })), /*#__PURE__*/React.createElement(OvlEyebrow, {
    style: {
      fontSize: 10,
      display: 'block',
      padding: '16px 10px 8px'
    }
  }, "Available commands"), filteredCmds.map((c, i) => /*#__PURE__*/React.createElement(CommandRow, {
    key: c.id,
    icon: c.icon,
    title: c.title,
    subtitle: c.subtitle,
    shortcut: c.shortcut,
    selected: sel === recents.length + i,
    onClick: () => onCommand(c.id)
  }))), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      height: 32,
      padding: '0 16px',
      background: 'var(--color-surface-container-low)',
      borderTop: '1px solid var(--color-outline-variant)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 16
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 4
    }
  }, /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 14,
      color: 'var(--color-outline)'
    }
  }, "keyboard_arrow_up"), /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 14,
      color: 'var(--color-outline)'
    }
  }, "keyboard_arrow_down"), /*#__PURE__*/React.createElement(OvlEyebrow, {
    style: {
      fontSize: 9
    }
  }, "Navigate")), /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 6
    }
  }, /*#__PURE__*/React.createElement(OvlKbd, null, "\u21A9"), /*#__PURE__*/React.createElement(OvlEyebrow, {
    style: {
      fontSize: 9
    }
  }, "Select"))), /*#__PURE__*/React.createElement(OvlEyebrow, {
    style: {
      fontSize: 9
    }
  }, "Bistro // V.01"))));
}
function HelpOverlay({
  onClose
}) {
  const sections = window.BistroData.shortcuts;
  return /*#__PURE__*/React.createElement("div", {
    onMouseDown: e => {
      if (e.target === e.currentTarget) onClose();
    },
    style: {
      position: 'absolute',
      inset: 0,
      zIndex: 40,
      background: 'rgba(0,0,0,0.6)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      width: 680,
      maxHeight: '80%',
      overflowY: 'auto',
      background: 'var(--color-surface-container)',
      boxShadow: 'var(--shadow-ambient)',
      padding: 32
    }
  }, /*#__PURE__*/React.createElement("h2", {
    style: {
      margin: '0 0 24px',
      fontFamily: 'var(--font-mono)',
      fontSize: 18,
      fontWeight: 700,
      color: 'var(--color-on-surface)'
    }
  }, "Keyboard Shortcuts"), Object.entries(sections).map(([title, rows]) => /*#__PURE__*/React.createElement("section", {
    key: title,
    style: {
      marginBottom: 28
    }
  }, /*#__PURE__*/React.createElement("h3", {
    style: {
      margin: '0 0 14px',
      fontFamily: 'var(--font-mono)',
      fontSize: 13,
      fontWeight: 700,
      letterSpacing: '0.15em',
      textTransform: 'uppercase',
      color: 'var(--color-on-surface)'
    }
  }, title), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 10
    }
  }, rows.map(([key, desc]) => /*#__PURE__*/React.createElement("div", {
    key: key,
    style: {
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center'
    }
  }, /*#__PURE__*/React.createElement("kbd", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 12,
      padding: '3px 10px',
      background: 'var(--color-surface-container-highest)',
      border: '1px solid var(--color-outline-variant)',
      color: 'var(--color-on-surface-variant)'
    }
  }, key), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 14,
      color: 'var(--color-on-surface)'
    }
  }, desc)))))), /*#__PURE__*/React.createElement("div", {
    style: {
      textAlign: 'center',
      marginTop: 8,
      paddingTop: 20,
      borderTop: '1px solid var(--color-outline-variant)',
      fontFamily: 'var(--font-sans)',
      fontSize: 13,
      color: 'var(--color-on-surface-variant)'
    }
  }, "Press ", /*#__PURE__*/React.createElement("kbd", {
    style: {
      fontFamily: 'var(--font-mono)',
      background: 'var(--color-surface-container)',
      padding: '1px 6px'
    }
  }, "?"), " or ", /*#__PURE__*/React.createElement("kbd", {
    style: {
      fontFamily: 'var(--font-mono)',
      background: 'var(--color-surface-container)',
      padding: '1px 6px'
    }
  }, "Esc"), " to close")));
}
window.CommandPalette = CommandPalette;
window.HelpOverlay = HelpOverlay;
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/bistro-app/Overlays.jsx", error: String((e && e.message) || e) }); }

// ui_kits/bistro-app/OverviewScreen.jsx
try { (() => {
// Bistro UI kit — PR Overview. Mirrors PR-Overview (Minimal Refined).
const {
  Eyebrow: OvEyebrow,
  CommentCard,
  Button: OvButton,
  StatusDot: OvDot
} = window.BistroDesignSystem_8e22c3;
function MetaCol({
  label,
  value,
  mono = true,
  color
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 8
    }
  }, /*#__PURE__*/React.createElement(OvEyebrow, {
    style: {
      fontSize: 11
    }
  }, label), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: mono ? 'var(--font-mono)' : 'var(--font-sans)',
      fontSize: 14,
      color: color || 'var(--color-on-surface)'
    }
  }, value));
}
function OverviewScreen({
  onViewDiff
}) {
  const pr = window.BistroData.pr;
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: '100%',
      overflowY: 'auto',
      background: 'var(--color-surface)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      maxWidth: 1700,
      margin: '0 auto'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1,
      minWidth: 0,
      padding: '40px 56px 64px'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 16,
      marginBottom: 28
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 13,
      fontWeight: 700,
      letterSpacing: '0.1em',
      color: 'var(--color-primary)'
    }
  }, "PR-", pr.number), /*#__PURE__*/React.createElement("span", {
    style: {
      flex: 1,
      height: 1,
      background: 'var(--color-outline-variant)'
    }
  })), /*#__PURE__*/React.createElement("h1", {
    style: {
      margin: '0 0 32px',
      fontFamily: 'var(--font-sans)',
      fontSize: 52,
      fontWeight: 800,
      lineHeight: 1.05,
      letterSpacing: '-0.03em',
      color: 'var(--color-on-surface)',
      maxWidth: 1100
    }
  }, pr.title), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      gap: 48,
      flexWrap: 'wrap',
      marginBottom: 36
    }
  }, /*#__PURE__*/React.createElement(MetaCol, {
    label: "Author",
    value: '@' + pr.author
  }), /*#__PURE__*/React.createElement(MetaCol, {
    label: "Date",
    value: pr.date
  }), /*#__PURE__*/React.createElement(MetaCol, {
    label: "Source",
    value: /*#__PURE__*/React.createElement("span", {
      style: {
        background: 'var(--color-surface-container)',
        padding: '3px 8px'
      }
    }, pr.source)
  }), /*#__PURE__*/React.createElement(MetaCol, {
    label: "Target",
    value: /*#__PURE__*/React.createElement("span", {
      style: {
        background: 'var(--color-surface-container)',
        padding: '3px 8px'
      }
    }, pr.target)
  }), /*#__PURE__*/React.createElement(MetaCol, {
    label: "Files",
    value: pr.files
  }), /*#__PURE__*/React.createElement(MetaCol, {
    label: "Added",
    value: '+' + pr.added,
    color: "var(--color-diff-add-text)"
  }), /*#__PURE__*/React.createElement(MetaCol, {
    label: "Removed",
    value: '-' + pr.removed,
    color: "var(--color-diff-remove-text)"
  })), /*#__PURE__*/React.createElement("div", {
    style: {
      border: '1px solid var(--color-primary)',
      padding: '28px 32px',
      marginBottom: 24
    }
  }, /*#__PURE__*/React.createElement("p", {
    style: {
      margin: 0,
      fontFamily: 'var(--font-sans)',
      fontSize: 18,
      lineHeight: 1.6,
      color: 'var(--color-on-surface)'
    }
  }, pr.description)), /*#__PURE__*/React.createElement("div", {
    style: {
      background: 'var(--color-surface-container-lowest)',
      border: '1px solid var(--color-outline-variant)',
      padding: '28px 32px',
      marginBottom: 44
    }
  }, /*#__PURE__*/React.createElement(OvEyebrow, {
    style: {
      fontSize: 12
    }
  }, "Technical Impact"), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 14,
      marginTop: 18
    }
  }, pr.impact.map((t, i) => /*#__PURE__*/React.createElement("div", {
    key: i,
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 12
    }
  }, /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 18,
      color: 'var(--color-primary)'
    }
  }, "check"), /*#__PURE__*/React.createElement("span", {
    style: {
      fontFamily: 'var(--font-mono)',
      fontSize: 14,
      color: 'var(--color-on-surface-variant)'
    },
    dangerouslySetInnerHTML: {
      __html: t.replace(/`([^`]+)`/g, '<span style="background:var(--color-surface-container);padding:1px 5px">$1</span>')
    }
  }))))), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      marginBottom: 20
    }
  }, /*#__PURE__*/React.createElement("h2", {
    style: {
      margin: 0,
      fontFamily: 'var(--font-mono)',
      fontSize: 15,
      letterSpacing: '0.18em',
      textTransform: 'uppercase',
      color: 'var(--color-on-surface)',
      fontWeight: 700
    }
  }, "Recent Comments"), /*#__PURE__*/React.createElement(OvEyebrow, {
    style: {
      fontSize: 11
    }
  }, "03 Unresolved")), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      gap: 16,
      marginBottom: 36
    }
  }, pr.comments.map((c, i) => /*#__PURE__*/React.createElement(CommentCard, {
    key: i,
    author: c.author,
    time: c.time,
    actions: c.actions ? /*#__PURE__*/React.createElement(React.Fragment, null, /*#__PURE__*/React.createElement(OvButton, {
      variant: "ghost",
      size: "sm"
    }, "Reply"), /*#__PURE__*/React.createElement(OvButton, {
      variant: "ghost",
      size: "sm"
    }, "Resolve")) : undefined
  }, c.body))), /*#__PURE__*/React.createElement("button", {
    type: "button",
    onClick: onViewDiff,
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 12,
      padding: '14px 20px',
      background: 'var(--color-surface-container)',
      border: 'none',
      cursor: 'pointer',
      fontFamily: 'var(--font-mono)',
      fontSize: 13,
      letterSpacing: '0.1em',
      textTransform: 'uppercase',
      color: 'var(--color-on-surface-variant)'
    }
  }, /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 18
    }
  }, "keyboard_command_key"), "Press [Space] on any file to view diff")), /*#__PURE__*/React.createElement("div", {
    style: {
      width: 180,
      flexShrink: 0,
      padding: '40px 24px',
      display: 'flex',
      flexDirection: 'column',
      gap: 36,
      alignItems: 'center'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      gap: 10
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      width: 44,
      height: 44,
      border: '1px solid var(--color-primary)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center'
    }
  }, /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 22,
      color: 'var(--color-primary)'
    }
  }, "autorenew")), /*#__PURE__*/React.createElement(OvEyebrow, {
    tone: "primary",
    style: {
      fontSize: 10,
      textAlign: 'center'
    }
  }, "Checks Running")), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      gap: 10
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      width: 44,
      height: 44,
      border: '1px solid var(--color-outline-variant)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center'
    }
  }, /*#__PURE__*/React.createElement("span", {
    className: "material-symbols-outlined",
    style: {
      fontSize: 22,
      color: 'var(--color-outline)'
    }
  }, "person")), /*#__PURE__*/React.createElement(OvEyebrow, {
    style: {
      fontSize: 10,
      textAlign: 'center'
    }
  }, "Awaiting Review")))));
}
window.OverviewScreen = OverviewScreen;
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/bistro-app/OverviewScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/bistro-app/SettingsScreen.jsx
try { (() => {
// Bistro UI kit — Settings. Mirrors Settings (Minimal).
const {
  Eyebrow: SetEyebrow,
  Switch: SetSwitch,
  SegmentedControl: SetSeg,
  Input: SetInput,
  Button: SetButton,
  Kbd: SetKbd
} = window.BistroDesignSystem_8e22c3;
function SettingRow({
  title,
  desc,
  control
}) {
  return /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'flex-start',
      justifyContent: 'space-between',
      gap: 32,
      padding: '20px 0',
      borderBottom: '1px solid var(--color-outline-variant)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      maxWidth: 520
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 15,
      fontWeight: 600,
      color: 'var(--color-on-surface)'
    }
  }, title), desc && /*#__PURE__*/React.createElement("div", {
    style: {
      fontFamily: 'var(--font-sans)',
      fontSize: 13,
      color: 'var(--color-on-surface-variant)',
      marginTop: 4,
      lineHeight: 1.5
    }
  }, desc)), /*#__PURE__*/React.createElement("div", {
    style: {
      flexShrink: 0
    }
  }, control));
}
function SettingsSection({
  eyebrow,
  title,
  children
}) {
  return /*#__PURE__*/React.createElement("section", {
    style: {
      marginBottom: 56
    }
  }, /*#__PURE__*/React.createElement(SetEyebrow, {
    tone: "primary",
    style: {
      display: 'block',
      marginBottom: 8
    }
  }, eyebrow), /*#__PURE__*/React.createElement("h2", {
    style: {
      margin: '0 0 8px',
      fontFamily: 'var(--font-sans)',
      fontSize: 30,
      fontWeight: 400,
      letterSpacing: '-0.03em',
      color: 'var(--color-on-surface)'
    }
  }, title), /*#__PURE__*/React.createElement("div", null, children));
}
function SettingsScreen() {
  const [theme, setTheme] = React.useState('light');
  const [view, setView] = React.useState('grouped');
  const [autoSync, setAutoSync] = React.useState(true);
  const [offline, setOffline] = React.useState(true);
  const [uiFont, setUiFont] = React.useState('Geist');
  return /*#__PURE__*/React.createElement("div", {
    style: {
      height: '100%',
      overflowY: 'auto',
      background: 'var(--color-surface-container-lowest)'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      maxWidth: 1152,
      margin: '0 auto',
      padding: '80px 80px 120px'
    }
  }, /*#__PURE__*/React.createElement("header", {
    style: {
      marginBottom: 64
    }
  }, /*#__PURE__*/React.createElement(SetEyebrow, {
    tone: "primary",
    style: {
      display: 'block',
      marginBottom: 16
    }
  }, "Workspace / Global"), /*#__PURE__*/React.createElement("h1", {
    style: {
      margin: 0,
      fontFamily: 'var(--font-sans)',
      fontSize: 60,
      fontWeight: 400,
      letterSpacing: '-0.05em',
      lineHeight: 1,
      color: 'var(--color-on-surface)'
    }
  }, "Settings")), /*#__PURE__*/React.createElement(SettingsSection, {
    eyebrow: "01 / Appearance",
    title: "Appearance"
  }, /*#__PURE__*/React.createElement(SettingRow, {
    title: "Theme",
    desc: "Dark mode is the developer default; light is the designed primary.",
    control: /*#__PURE__*/React.createElement(SetSeg, {
      value: theme,
      onChange: setTheme,
      options: [{
        value: 'light',
        label: 'Light'
      }, {
        value: 'dark',
        label: 'Dark'
      }, {
        value: 'system',
        label: 'System'
      }]
    })
  }), /*#__PURE__*/React.createElement(SettingRow, {
    title: "UI font",
    desc: "Resolves through --app-ui-font. Changes apply live.",
    control: /*#__PURE__*/React.createElement("div", {
      style: {
        width: 220
      }
    }, /*#__PURE__*/React.createElement(SetInput, {
      value: uiFont,
      onChange: e => setUiFont(e.target.value),
      icon: "text_fields"
    }))
  }), /*#__PURE__*/React.createElement(SettingRow, {
    title: "Inbox view",
    desc: "Flat list, or grouped by repository with foldable sections.",
    control: /*#__PURE__*/React.createElement(SetSeg, {
      value: view,
      onChange: setView,
      options: [{
        value: 'flat',
        label: 'Flat'
      }, {
        value: 'grouped',
        label: 'Grouped'
      }]
    })
  })), /*#__PURE__*/React.createElement(SettingsSection, {
    eyebrow: "02 / Sync",
    title: "Sync behavior"
  }, /*#__PURE__*/React.createElement(SettingRow, {
    title: "Auto-sync",
    desc: "Keep the local SQLite cache warm by syncing watched repos every 60 seconds.",
    control: /*#__PURE__*/React.createElement(SetSwitch, {
      checked: autoSync,
      onChange: e => setAutoSync(e.target.checked)
    })
  }), /*#__PURE__*/React.createElement(SettingRow, {
    title: "Offline queue",
    desc: "Queue approvals and comments locally when disconnected; replay on reconnect.",
    control: /*#__PURE__*/React.createElement(SetSwitch, {
      checked: offline,
      onChange: e => setOffline(e.target.checked)
    })
  })), /*#__PURE__*/React.createElement(SettingsSection, {
    eyebrow: "03 / Keyboard",
    title: "Key bindings"
  }, /*#__PURE__*/React.createElement(SettingRow, {
    title: "Approve",
    desc: "Double-tap to confirm. Remap any binding; the footer legend updates everywhere.",
    control: /*#__PURE__*/React.createElement("span", {
      style: {
        display: 'flex',
        gap: 6
      }
    }, /*#__PURE__*/React.createElement(SetKbd, null, "A"), /*#__PURE__*/React.createElement(SetKbd, null, "A"))
  }), /*#__PURE__*/React.createElement(SettingRow, {
    title: "Command palette",
    control: /*#__PURE__*/React.createElement(SetKbd, {
      tone: "primary"
    }, ":")
  }), /*#__PURE__*/React.createElement(SettingRow, {
    title: "Comment on line",
    control: /*#__PURE__*/React.createElement(SetKbd, null, "C")
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      marginTop: 24
    }
  }, /*#__PURE__*/React.createElement(SetButton, {
    variant: "outline",
    icon: "restart_alt"
  }, "Reset to defaults")))));
}
window.SettingsScreen = SettingsScreen;
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/bistro-app/SettingsScreen.jsx", error: String((e && e.message) || e) }); }

// ui_kits/bistro-app/data.js
try { (() => {
// Bistro UI kit — shared mock data. Assigned to window for cross-script use.
window.BistroData = {
  repos: [{
    key: 'core-engine / production',
    accent: true,
    prs: [{
      number: 1042,
      title: 'feat: implement zero-radius architecture for canvas nodes',
      author: 'alice',
      time: '2m ago',
      ci: 'success'
    }, {
      number: 1041,
      title: 'fix: memory leak in viewport observer cleanup',
      author: 'bob_dev',
      time: '15m ago',
      ci: 'failure'
    }, {
      number: 1039,
      title: 'refactor: migrate tailwind config to typescript',
      author: 'charlie',
      time: '1h ago',
      ci: 'running'
    }]
  }, {
    key: 'internal-tools / registry',
    accent: false,
    prs: [{
      number: 982,
      title: 'chore: update dependency graph for v2.4.0',
      author: 'system',
      time: '4h ago',
      ci: 'success'
    }]
  }, {
    key: 'legacy / monolith-v1',
    accent: false,
    collapsed: true,
    prs: [{
      number: 871,
      title: 'fix: patch CVE-2023-9812 context leakage',
      author: 'sarah_dev',
      time: '1d ago',
      ci: 'success'
    }, {
      number: 864,
      title: 'docs: archive deprecated BufferManager API',
      author: 'alex_mercer',
      time: '2d ago',
      ci: 'unknown'
    }]
  }],
  // The PR opened in overview / diff
  pr: {
    number: 2048,
    title: 'refactor: core/engine logic for multi-tenant isolation',
    author: 'alex_mercer',
    date: '2023-10-24 14:02:11',
    source: 'feat/isolation-v2',
    target: 'main',
    files: 12,
    added: 842,
    removed: 156,
    description: "This PR implements the structural changes required to support hard isolation between tenants at the engine layer. It removes the shared memory buffer and introduces a keyed proxy pattern for all state access.",
    impact: ['Decreases cold-boot latency by 14%', 'Fixes CVE-2023-9812 context leakage', 'Deprecates old `BufferManager` instance'],
    comments: [{
      author: 'sarah_dev',
      time: '2h ago',
      body: "Should we consider adding a fallback for legacy tenants that haven't migrated to the new key structure? I'm worried about breaking the SDK.",
      actions: true
    }, {
      author: 'system_ci',
      time: '5h ago',
      body: 'Unit tests passed. Integration tests pending for pkg/driver/v2.',
      actions: false
    }],
    checks: 'running'
  },
  diff: {
    file: 'src/core/monolith_engine.rs',
    lang: 'RUST',
    added: 24,
    removed: 12,
    files: [{
      path: 'src/core/monolith_engine.rs',
      active: true
    }, {
      path: 'src/ui/layout_grid.tsx'
    }, {
      path: 'config/system_params.yaml'
    }, {
      path: 'tests/unit/protocol_tests.rs'
    }, {
      path: 'scripts/deploy_monolith.sh'
    }],
    lines: [{
      o: 41,
      n: 41,
      t: 'context',
      c: 'pub fn initialize_kernel(&mut self) -> Result<(), EngineError> {'
    }, {
      o: 42,
      n: 42,
      t: 'context',
      c: '    log::info!("Starting Monolith Kernel...");'
    }, {
      n: 43,
      t: 'remove',
      c: '    self.status = EngineStatus::Initializing;'
    }, {
      o: 43,
      t: 'add',
      c: '    self.set_state(EngineStatus::Initializing)?;'
    }, {
      o: 44,
      t: 'add',
      focused: true,
      c: '    self.verify_integrity_hash()?;'
    }, {
      o: 45,
      n: 45,
      t: 'context',
      c: '    self.runtime.spawn(async move {'
    }, {
      o: 46,
      n: 46,
      t: 'context',
      c: '        match self.execute_bootstrap().await {'
    }, {
      o: 47,
      t: 'add',
      c: '            Ok(_) => log::debug!("Bootstrap sequence verified."),'
    }, {
      o: 48,
      t: 'add',
      c: '            Err(e) => {'
    }, {
      o: 49,
      t: 'add',
      c: '                log::error!("Critical failure during bootstrap: {:?}", e);'
    }, {
      o: 50,
      t: 'add',
      c: '                std::process::exit(1);'
    }, {
      o: 51,
      t: 'add',
      c: '            }'
    }, {
      o: 52,
      n: 52,
      t: 'context',
      c: '        }'
    }, {
      o: 53,
      n: 53,
      t: 'context',
      c: '    });'
    }]
  },
  commands: [{
    id: 'help',
    icon: 'keyboard',
    title: 'Show keyboard shortcuts',
    shortcut: ['?']
  }, {
    id: 'settings',
    icon: 'settings',
    title: 'Open project settings',
    shortcut: ['⌘', ',']
  }, {
    id: 'sync',
    icon: 'sync',
    title: 'Sync now',
    subtitle: 'Force sync all watched repos'
  }, {
    id: 'clear',
    icon: 'filter_alt_off',
    title: 'Clear all filters and search'
  }, {
    id: 'repos',
    icon: 'source_environment',
    title: 'Open repository picker',
    subtitle: 'Discover and watch repositories'
  }, {
    id: 'focus-list',
    icon: 'list_alt',
    title: 'Focus PR list'
  }, {
    id: 'about',
    icon: 'info',
    title: 'About BISTRO'
  }],
  shortcuts: {
    Navigation: [['J / K', 'Move down / up'], ['G G', 'Jump to top'], ['Shift+G', 'Jump to bottom'], ['N / P', 'Next / prev file'], ['H J K L', 'Scroll diff']],
    Actions: [['Enter', 'Open PR'], ['A A', 'Approve (double-tap)'], ['R', 'Request changes'], ['C', 'Comment on line'], ['Esc', 'Back / close']],
    'Search & Commands': [[':', 'Command palette'], ['/', 'Search'], ['?', 'Keyboard shortcuts'], ['⌘ ,', 'Settings'], ['Space', 'Toggle fold / view diff']]
  }
};
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/bistro-app/data.js", error: String((e && e.message) || e) }); }

__ds_ns.Avatar = __ds_scope.Avatar;

__ds_ns.Badge = __ds_scope.Badge;

__ds_ns.Button = __ds_scope.Button;

__ds_ns.Eyebrow = __ds_scope.Eyebrow;

__ds_ns.IconButton = __ds_scope.IconButton;

__ds_ns.Kbd = __ds_scope.Kbd;

__ds_ns.StatusDot = __ds_scope.StatusDot;

__ds_ns.CommandRow = __ds_scope.CommandRow;

__ds_ns.LineFocusChip = __ds_scope.LineFocusChip;

__ds_ns.Toast = __ds_scope.Toast;

__ds_ns.Checkbox = __ds_scope.Checkbox;

__ds_ns.Input = __ds_scope.Input;

__ds_ns.SegmentedControl = __ds_scope.SegmentedControl;

__ds_ns.Switch = __ds_scope.Switch;

__ds_ns.Panel = __ds_scope.Panel;

__ds_ns.SystemBar = __ds_scope.SystemBar;

__ds_ns.Toolbar = __ds_scope.Toolbar;

__ds_ns.CommentCard = __ds_scope.CommentCard;

__ds_ns.DiffLine = __ds_scope.DiffLine;

__ds_ns.PRRow = __ds_scope.PRRow;

})();
