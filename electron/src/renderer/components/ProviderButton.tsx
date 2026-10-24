interface ProviderButtonProps {
  provider: 'github' | 'gitlab' | 'gitea';
  icon: string;
  label: string;
  keyHint?: string | undefined;
  availabilityLabel?: string | undefined;
  onClick: () => void;
  disabled?: boolean;
  autoFocus?: boolean;
}

/**
 * Single provider auth button. Outline on surface-container-lowest, hover
 * fills to surface-container-highest; the right side shows the effective key
 * or, for providers that are not available yet, an availability label.
 */
export function ProviderButton({
  provider,
  icon,
  label,
  keyHint,
  availabilityLabel,
  onClick,
  disabled = false,
  autoFocus = false,
}: ProviderButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      // biome-ignore lint/a11y/noAutofocus: the sign-in screen's primary action takes focus on mount.
      autoFocus={autoFocus}
      data-provider={provider}
      className="w-full h-14 flex items-center justify-between px-5 bg-surface-container-lowest border border-outline hover:bg-surface-container-highest transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-surface-container-lowest disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:bg-surface-container-lowest"
    >
      <div className="flex items-center gap-4">
        <span className="material-symbols-outlined text-on-surface">
          {icon}
        </span>
        <span className="font-bold text-sm tracking-tight text-on-surface">
          {label}
        </span>
      </div>
      {availabilityLabel !== undefined ? (
        <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
          {availabilityLabel}
        </span>
      ) : (
        keyHint !== undefined &&
        keyHint !== '' && (
          <span className="font-mono text-[10px] text-outline px-1.5 py-0.5 border border-outline-variant/50">
            {keyHint}
          </span>
        )
      )}
    </button>
  );
}
