import * as React from 'react';

/**
 * The Bistro logo symbol — a 3D card holding a "B" with a blue cursor block.
 *
 * @startingPoint section="Brand" subtitle="The Bistro logo mark + lockup" viewport="700x260"
 */
export interface BistroMarkProps
  extends React.SVGProps<SVGSVGElement> {
  /** Edge length in px. @default 64 */
  size?: number;
  /** default = white card / dark line; inverse = dark card / light B; mono = single-color (currentColor). @default "default" */
  variant?: 'default' | 'inverse' | 'mono';
  /** Cursor-block color (ignored in mono). @default Bistro blue */
  accent?: string;
}

export function BistroMark(props: BistroMarkProps): React.JSX.Element;
