import { openUrlInBrowser } from '../utils/openUrl';

interface ExternalLinkProps {
  href?: string | undefined;
  children: React.ReactNode;
  className?: string | undefined;
}

/**
 * The single renderer-owned external-link primitive. Prevents in-app navigation
 * and opens HTTPS URLs in the OS browser via the preload bridge (main validates
 * HTTPS-only — main/index.ts). Non-https / relative / javascript: hrefs are inert.
 * The shared URL service also applies the benchmark's external-write guard.
 */
export function ExternalLink({
  href,
  children,
  className,
}: ExternalLinkProps): React.JSX.Element {
  function handleClick(event: React.MouseEvent<HTMLAnchorElement>): void {
    event.preventDefault();
    if (href === undefined || !href.startsWith('https://')) return;
    void openUrlInBrowser(href);
  }

  return (
    <a
      href={href}
      onClick={handleClick}
      rel="noopener noreferrer"
      className={className}
    >
      {children}
    </a>
  );
}
