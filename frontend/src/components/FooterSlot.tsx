import React from 'react';

/**
 * Footer slot system.
 *
 * Pages can render into the footer's left or center regions via {@link FooterLeftSlot}
 * / {@link FooterCenterSlot}. When a slot is empty, `AppFooter` falls back to a
 * mode-aware legend derived from the effective keybindings.
 *
 * The right-hand region is reserved for global chrome (status, shortcuts, settings)
 * and is not slottable.
 *
 * Implementation: a tiny pub/sub in context — setters bump a version counter so
 * only the footer re-renders when content changes, not the whole tree.
 */

type SlotId = 'left' | 'center';

interface FooterSlotContextValue {
  content: Record<SlotId, React.ReactNode | null>;
  setContent: (id: SlotId, node: React.ReactNode | null) => void;
}

const FooterSlotContext = React.createContext<FooterSlotContextValue | null>(
  null,
);

export function FooterSlotProvider({
  children,
}: {
  children: React.ReactNode;
}): React.ReactElement {
  const [content, setContentState] = React.useState<
    Record<SlotId, React.ReactNode | null>
  >({ left: null, center: null });

  const setContent = React.useCallback(
    (id: SlotId, node: React.ReactNode | null) => {
      setContentState((prev) => {
        if (prev[id] === node) return prev;
        return { ...prev, [id]: node };
      });
    },
    [],
  );

  const value = React.useMemo(
    () => ({ content, setContent }),
    [content, setContent],
  );

  return (
    <FooterSlotContext.Provider value={value}>
      {children}
    </FooterSlotContext.Provider>
  );
}

function useFooterSlot(): FooterSlotContextValue {
  const ctx = React.useContext(FooterSlotContext);
  if (!ctx) {
    throw new Error(
      'FooterSlot components must be used inside <FooterSlotProvider>',
    );
  }
  return ctx;
}

/** Read-only access for the footer renderer. */
export function useFooterSlotContent(): Record<SlotId, React.ReactNode | null> {
  return useFooterSlot().content;
}

interface SlotProps {
  children: React.ReactNode;
}

function FooterSlotImpl({ id, children }: { id: SlotId } & SlotProps): null {
  const { setContent } = useFooterSlot();
  React.useEffect(() => {
    setContent(id, children);
    return () => setContent(id, null);
  }, [id, children, setContent]);
  return null;
}

/** Mount anywhere in a page to populate the footer's left region. */
export function FooterLeftSlot({
  children,
}: SlotProps): React.ReactElement | null {
  return <FooterSlotImpl id="left">{children}</FooterSlotImpl>;
}

/** Mount anywhere in a page to populate the footer's center region. */
export function FooterCenterSlot({
  children,
}: SlotProps): React.ReactElement | null {
  return <FooterSlotImpl id="center">{children}</FooterSlotImpl>;
}
