import { type ReactElement, useMemo } from 'react';
import { NAVIGATION_STYLES, type NavigationStyle } from '../../types/settings';
import { useStore } from '../hooks/useStore';
import { cn } from '../lib/utils';
import { settingsStore } from '../store/settings';
import { getEffectiveBindings } from '../utils/keybindings';
import { findActionKeys } from '../utils/legendPresets';
import { Kbd } from './ui/kbd';

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
  /** `grid` is the Settings presentation; onboarding stacks the cards. */
  readonly layout?: 'grid' | 'vertical';
}

/** Movement and action chips shown on the active vertical card. */
const ACTIVE_CARD_ACTION_IDS: readonly (readonly string[])[] = [
  ['list.down', 'list.downArrow'],
  ['list.up', 'list.upArrow'],
  ['list.top'],
  ['list.bottom'],
  ['list.open'],
  ['detail.approve'],
];

/** Mouse clicks must not take focus: a focused button would swallow Space. */
export const keepFocus = (event: { preventDefault: () => void }): void =>
  event.preventDefault();

export function NavigationStylePicker({
  selected,
  onSelect,
  layout = 'grid',
}: NavigationStylePickerProps): ReactElement {
  if (layout === 'vertical')
    return <VerticalPicker selected={selected} onSelect={onSelect} />;
  return (
    <fieldset
      aria-label="Navigation style"
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
            onMouseDown={keepFocus}
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

function VerticalPicker({
  selected,
  onSelect,
}: NavigationStylePickerProps): ReactElement {
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const chips = useMemo(() => {
    const bindings = getEffectiveBindings(selected, overrides);
    const keys = ACTIVE_CARD_ACTION_IDS.flatMap(
      (ids) => findActionKeys(bindings, ids) ?? [],
    );
    return [...new Set(keys)];
  }, [selected, overrides]);

  return (
    <fieldset aria-label="Navigation style" className="space-y-2 min-w-0">
      {NAVIGATION_STYLES.map((style) => {
        const card: NavigationStyleCard = NAVIGATION_STYLE_CARDS[style];
        const isSelected = style === selected;
        return (
          <button
            key={style}
            type="button"
            aria-pressed={isSelected}
            onMouseDown={keepFocus}
            onClick={() => onSelect(style)}
            className={cn(
              // `!`: the unlayered `button` reset in globals.css beats layered utilities.
              'grid w-full grid-cols-[auto_1fr] gap-x-3 items-baseline text-left px-4 py-3 border transition-colors duration-150 motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-surface',
              isSelected
                ? 'bg-surface-container-lowest! border-primary! border-l-4! py-4'
                : 'bg-surface-container-lowest! border-outline-variant/30! hover:border-outline!',
            )}
          >
            <span
              aria-hidden="true"
              className={cn(
                'size-2.5 border',
                isSelected
                  ? 'bg-primary border-primary'
                  : 'border-on-surface-variant',
              )}
            />
            <span className="flex items-baseline justify-between gap-3">
              <span
                className={cn(
                  'font-bold text-on-surface',
                  isSelected ? 'text-lg' : 'text-sm',
                )}
              >
                {card.label}
              </span>
              {isSelected && (
                <span className="mono-text text-[10px] text-primary uppercase tracking-widest">
                  Active
                </span>
              )}
            </span>
            <span className="col-start-2 mt-1 text-xs text-on-surface-variant leading-snug">
              {card.pitch}
            </span>
            {isSelected && chips.length > 0 && (
              <span className="col-start-2 mt-2 flex flex-wrap gap-1.5">
                {chips.map((keys) => (
                  <Kbd key={keys}>{keys}</Kbd>
                ))}
              </span>
            )}
            {isSelected && card.caveat !== undefined && (
              <span className="col-start-2 mt-2 mono-text text-[10px] text-outline leading-relaxed">
                {card.caveat}
              </span>
            )}
          </button>
        );
      })}
    </fieldset>
  );
}
