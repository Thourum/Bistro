import type React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useStore } from '../hooks/useStore';
import { useOverlays } from '../overlays/OverlayContext';
import { settingsStore, updateSettings } from '../store/settings';
import { getEffectiveBindings } from '../utils/keybindings';
import { resolveTourScreens } from '../utils/tourScreens';
import { addToast } from './ToastContainer';
import { Button } from './ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog';
import { Kbd } from './ui/kbd';

function persistTourComplete(): void {
  const { appearance } = settingsStore.state;
  if (appearance.tourComplete) return;
  void updateSettings({
    appearance: { ...appearance, tourComplete: true },
  }).match(
    () => undefined,
    (message) =>
      addToast({
        type: 'error',
        message: `Tour completion could not be saved: ${message}`,
        dismissible: true,
      }),
  );
}

/**
 * 5-screen first-run tour (spec §5.4). Shows once (`appearance.tourComplete`),
 * re-runnable via `:tour`. The flag is set on ANY exit — finishing screen 5,
 * Esc mid-tour, or the shell unmounting while open all persist it (Esc is
 * handled by the global overlay-close binding, which doesn't know about tour
 * completion, so this component observes the open→closed transition rather
 * than any specific close path).
 */
export function TourOverlay(): React.ReactElement | null {
  const { activeOverlay, openTour, closeOverlay } = useOverlays();
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const tourComplete = useStore(
    settingsStore,
    (s) => s.appearance.tourComplete,
  );
  const isOpen = activeOverlay === 'tour';
  const [screenIndex, setScreenIndex] = useState(0);
  const hasAutoOpened = useRef(false);

  // Auto-open once, right after onboarding completes (AuthedRouteShell only
  // mounts once authed+onboarded). Waits for any other overlay to close so it
  // never steals focus; the ref stops it re-opening a tour the user dismissed
  // before the completion write has round-tripped through settings.
  useEffect(() => {
    if (hasAutoOpened.current || tourComplete || activeOverlay !== null) return;
    hasAutoOpened.current = true;
    openTour();
  }, [tourComplete, activeOverlay, openTour]);

  // Cleanup fires on open→closed and on unmount-while-open alike.
  useEffect(() => {
    if (!isOpen) return;
    return () => {
      setScreenIndex(0);
      persistTourComplete();
    };
  }, [isOpen]);

  const screens = useMemo(
    () => resolveTourScreens(getEffectiveBindings(overrides)),
    [overrides],
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
