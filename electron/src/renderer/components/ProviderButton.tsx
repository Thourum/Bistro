import React from 'react';
import { Button } from './ui/button';
import { cn } from '../lib/utils';

interface ProviderButtonProps {
  provider: 'github' | 'gitlab' | 'gitea';
  icon: string;
  label: string;
  keyHint: string;
  onClick: () => void;
}

export function ProviderButton({
  provider,
  icon,
  label,
  keyHint,
  onClick,
}: ProviderButtonProps) {
  return (
    <Button
      onClick={onClick}
      variant="outline"
      className={cn(
        'w-full h-14 justify-between px-5',
        'bg-surface-container-lowest border-outline-variant',
        'hover:bg-surface-container-low'
      )}
      data-provider={provider}
    >
      <div className="flex items-center gap-4">
        <span className="material-symbols-outlined text-lg">{icon}</span>
        <span className="font-bold text-sm text-on-surface">{label}</span>
      </div>
      <span className="font-mono text-[10px] text-outline px-2 py-1 bg-surface-container border border-outline-variant">
        {keyHint}
      </span>
    </Button>
  );
}
