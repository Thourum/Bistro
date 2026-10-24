import type React from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useStore } from '../hooks/useStore';
import { useOverlays } from '../overlays/OverlayContext';
import { settingsStore } from '../store/settings';
import { getEffectiveBindings } from '../utils/keybindings';
import { resolveTourScreens } from '../utils/tourScreens';
import { Button } from './ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog';
import { Kbd } from './ui/kbd';

/**
 * 5-screen keyboard tour (spec §5.4), opened manually via `:tour`. It never
 * opens itself and never writes `appearance.tourComplete`: that flag records
 * the first-run rail's Hide/Close, so replaying the tour can't dismiss it.
 */
export function TourOverlay(): React.ReactElement | null {
  const { activeOverlay, closeOverlay } = useOverlays();
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const isOpen = activeOverlay === 'tour';
  const [screenIndex, setScreenIndex] = useState(0);

  // Restart from the first screen on every open→closed transition.
  useEffect(() => {
    if (!isOpen) return;
    return () => setScreenIndex(0);
  }, [isOpen]);

  const screens = useMemo(
    () => resolveTourScreens(getEffectiveBindings(style, overrides)),
    [style, overrides],
  );
  const screen = screens[screenIndex];
  const isLastScreen = screenIndex === screens.length - 1;

  const finish = (): void => {
    closeOverlay();
  };

  const advance = (): void => {
    if (isLastScreen) {
      finish();
      return;
    }
    setScreenIndex((i) => i + 1);
  };

  const back = (): void => {
    setScreenIndex((i) => Math.max(0, i - 1));
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>): void => {
    // Enter on a focused button already clicks it — don't advance twice.
    if (event.key === 'Enter' && event.target instanceof HTMLButtonElement) {
      return;
    }
    if (event.key === 'ArrowRight' || event.key === 'Enter') {
      event.preventDefault();
      advance();
    } else if (event.key === 'ArrowLeft') {
      event.preventDefault();
      back();
    }
  };

  if (screen === undefined) return null;

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && finish()}>
      <DialogContent className="max-w-lg" onKeyDown={handleKeyDown}>
        <DialogHeader>
          <DialogTitle>
            {screen.title}
            <span className="ml-2 font-mono text-[10px] text-outline-variant">
              {screenIndex + 1}/{screens.length}
            </span>
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-2 text-sm">
          {screen.keys.map((hint) => (
            <div
              key={`${hint.keys}-${hint.label}`}
              className="flex items-center justify-between"
            >
              <Kbd>{hint.keys}</Kbd>
              <span className="text-on-surface">
                {hint.label}
                {hint.context !== undefined && (
                  <span className="ml-1 text-outline-variant">
                    ({hint.context})
                  </span>
                )}
              </span>
            </div>
          ))}
        </div>

        <div className="flex items-center justify-between pt-2">
          <Button variant="outline" onClick={back} disabled={screenIndex === 0}>
            Back
          </Button>
          <Button onClick={advance}>{isLastScreen ? 'Finish' : 'Next'}</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
