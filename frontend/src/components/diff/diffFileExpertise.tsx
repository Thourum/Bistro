import type { ExpertiseFileData } from '../../types';
import { formatWhy } from '../../utils/expertise';

export function DiffFileExpertise({
  expertise,
  error,
}: {
  expertise: ExpertiseFileData | undefined;
  error: string | null;
}): React.ReactElement | null {
  if (error !== null) {
    return (
      <div className="border-b border-outline-variant/10 bg-surface-container-lowest px-4 py-1 font-mono text-[10px] uppercase tracking-widest text-error">
        Expertise unavailable: {error}
      </div>
    );
  }

  if (expertise === undefined || (expertise.experts ?? []).length === 0) {
    return null;
  }

  return (
    <details className="border-b border-outline-variant/10 bg-surface-container-lowest px-4 py-1 font-mono text-[10px] text-outline">
      <summary className="cursor-crosshair uppercase tracking-widest text-on-surface-variant hover:text-on-surface">
        Expertise why
      </summary>
      <ul className="mt-2 space-y-1 border-l border-outline-variant/30 pl-3">
        {(expertise.experts ?? []).map((expert) => (
          <li key={`${expertise.file_path}-${expert.author_email}`}>
            {formatWhy(expert)}
          </li>
        ))}
      </ul>
      {expertise.your_rank !== null && (
        <p className="mt-2 text-primary">Your rank: #{expertise.your_rank}</p>
      )}
    </details>
  );
}
