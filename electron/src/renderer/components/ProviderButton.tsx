interface ProviderButtonProps {
  provider: 'github' | 'gitlab' | 'gitea';
  icon: string;
  label: string;
  keyHint: string;
  onClick: () => void;
  disabled?: boolean;
}

/**
 * Single provider auth button. Matches the row layout in
 * `bistro_login/code.html`: outline on surface-container-lowest, hover fills
 * to surface-container-highest, mono key-hint chip on the right.
 */
export function ProviderButton({
  provider,
  icon,
  label,
  keyHint,
  onClick,
  disabled = false,
}: ProviderButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      data-provider={provider}
      className="w-full h-14 flex items-center justify-between px-5 bg-surface-container-lowest border border-outline hover:bg-surface-container-highest transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
    >
      <div className="flex items-center gap-4">
        <span className="material-symbols-outlined text-on-surface">
          {icon}
        </span>
        <span className="font-bold text-sm tracking-tight text-on-surface">
          {label}
        </span>
      </div>
      <div className="font-mono text-[10px] text-outline flex items-center gap-2">
        <span className="px-1.5 py-0.5 border border-outline-variant/50">
          {keyHint}
        </span>
        <span className="uppercase opacity-60">Key Hint</span>
      </div>
    </button>
  );
}
