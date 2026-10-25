import type { ReactElement } from 'react';
import { NAVIGATION_STYLES, type NavigationStyle } from '../../types/settings';
import { cn } from '../lib/utils';

interface NavigationStyleCard {
  readonly label: string;
  readonly pitch: string;
  readonly caveat?: string;
}

export const NAVIGATION_STYLE_CARDS = {
  vim: { label: 'Vim', pitch: 'h j k l, g g, G — modal muscle memory.' },
  emacs: {
    label: 'Emacs',
    pitch: 'C-n / C-p to move, C-c C-<key> for actions.',
    caveat:
      'On Windows and Linux, C-c and C-x act as Bistro prefixes instead of copy and cut outside text fields.',
  },
  standard: {
    label: 'Standard',
    pitch: 'Arrows, Home / End, Page Up / Down, and the mouse.',
  },
} satisfies Record<NavigationStyle, NavigationStyleCard>;

interface NavigationStylePickerProps {
  readonly selected: NavigationStyle;
  readonly onSelect: (style: NavigationStyle) => void;
}

export function NavigationStylePicker({
  selected,
  onSelect,
}: NavigationStylePickerProps): ReactElement {
  return (
    <fieldset
      aria-label="Navigation style"
      data-navigation-style-picker
      className="grid grid-cols-1 md:grid-cols-3 gap-4 min-w-0"
    >
      {NAVIGATION_STYLES.map((style) => {
        const card: NavigationStyleCard = NAVIGATION_STYLE_CARDS[style];
        const isSelected = style === selected;
        return (
          <button
            key={style}
            type="button"
            aria-pressed={isSelected}
            // Mouse clicks must not take focus: a focused card would swallow Space.
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => onSelect(style)}
            className={cn(
              // `!`: the unlayered `button` reset in globals.css beats layered utilities.
              'flex flex-col items-start text-left p-6 border transition-colors duration-150 motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-surface',
              isSelected
                ? 'bg-surface-container-low! border-primary!'
                : 'bg-surface-container-lowest! border-outline-variant/30! hover:border-outline!',
            )}
          >
            <span className="flex w-full items-center justify-between gap-3">
              <span
                className={cn(
                  'mono-text text-xs font-bold uppercase tracking-widest',
                  isSelected ? 'text-primary' : 'text-on-surface',
                )}
              >
                {card.label}
              </span>
              <span
                aria-hidden="true"
                className={cn(
                  'material-symbols-outlined text-base',
                  isSelected ? 'text-primary' : 'text-outline-variant',
                )}
              >
                {isSelected ? 'radio_button_checked' : 'radio_button_unchecked'}
              </span>
            </span>
            <span className="mt-3 text-sm text-on-surface-variant leading-snug">
              {card.pitch}
            </span>
            {card.caveat !== undefined && (
              <span className="mt-auto pt-3 mono-text text-[10px] text-outline leading-relaxed">
                {card.caveat}
              </span>
            )}
          </button>
        );
      })}
    </fieldset>
  );
}
