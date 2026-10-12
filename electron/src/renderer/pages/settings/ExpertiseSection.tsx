import { fromPromise } from 'neverthrow';
import { useEffect, useState } from 'react';
import { addToast } from '../../components/ToastContainer';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { Textarea } from '../../components/ui/textarea';
import { useStore } from '../../hooks/useStore';
import { useWebSocket } from '../../hooks/useWebSocket';
import { appStore } from '../../store';
import type { UIState } from '../../store/ui';
import type { ExpertiseSettingsPayload } from '../../types';
import { Section } from './SettingsComponents';

const DEFAULT_MIN_SURVIVING_LINES = 50;

function splitListInput(value: string): string[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

function joinListInput(values: readonly string[]): string {
  return values.join('\n');
}

export function expertiseSettingsPayloadFromInputs(
  aliasInput: string,
  minSurvivingLines: number,
  excludeInputs: Record<string, string>,
): ExpertiseSettingsPayload {
  return {
    alias_emails: splitListInput(aliasInput),
    min_surviving_lines: minSurvivingLines,
    excludes: Object.fromEntries(
      Object.entries(excludeInputs).map(
        ([repoId, value]) => [repoId, splitListInput(value)] as const,
      ),
    ),
  };
}

function settingsOrDefault(
  settings: ExpertiseSettingsPayload | null,
): ExpertiseSettingsPayload {
  return {
    alias_emails: settings?.alias_emails ?? [],
    min_surviving_lines:
      settings?.min_surviving_lines ?? DEFAULT_MIN_SURVIVING_LINES,
    excludes: settings?.excludes ?? {},
  };
}

export function ExpertiseSection() {
  const { request } = useWebSocket();
  const repos = useStore(appStore, (s: UIState) => s.repos);
  const storedSettings = useStore(
    appStore,
    (s: UIState) => s.expertiseSettings,
  );
  const settings = settingsOrDefault(storedSettings);
  const [aliasInput, setAliasInput] = useState(
    joinListInput(settings.alias_emails),
  );
  const [minLinesInput, setMinLinesInput] = useState(
    String(settings.min_surviving_lines),
  );
  const [excludeInputs, setExcludeInputs] = useState<Record<string, string>>(
    () =>
      Object.fromEntries(
        Object.entries(settings.excludes).map(([repoId, globs]) => [
          repoId,
          joinListInput(globs),
        ]),
      ),
  );
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    void fromPromise(request('GET_EXPERTISE_SETTINGS', {}), (err) => err).match(
      () => undefined,
      (err) => {
        addToast({
          type: 'error',
          message: `Loading expertise settings failed: ${err instanceof Error ? err.message : 'sync engine unreachable'}`,
          dismissible: true,
        });
      },
    );
  }, [request]);

  useEffect(() => {
    if (storedSettings === null) return;
    setAliasInput(joinListInput(storedSettings.alias_emails));
    setMinLinesInput(String(storedSettings.min_surviving_lines));
    setExcludeInputs(
      Object.fromEntries(
        Object.entries(storedSettings.excludes).map(([repoId, globs]) => [
          repoId,
          joinListInput(globs),
        ]),
      ),
    );
  }, [storedSettings]);

  const watchedRepos = repos.filter((repo) => repo.watched);

  const save = (): void => {
    const trimmedMinLinesInput = minLinesInput.trim();
    const minLines = Number(trimmedMinLinesInput);
    if (
      trimmedMinLinesInput === '' ||
      !Number.isSafeInteger(minLines) ||
      minLines <= 0
    ) {
      addToast({
        type: 'error',
        message: 'Minimum surviving lines must be a positive whole number.',
        dismissible: true,
      });
      return;
    }

    const payload = expertiseSettingsPayloadFromInputs(
      aliasInput,
      minLines,
      excludeInputs,
    );

    setIsSaving(true);
    void fromPromise(
      request('SET_EXPERTISE_SETTINGS', payload),
      (err) => err,
    ).match(
      () => {
        setIsSaving(false);
        appStore.setState((s) => ({ ...s, expertiseSettings: payload }));
        addToast({
          type: 'success',
          message: 'Expertise settings saved.',
          dismissible: true,
        });
      },
      (err) => {
        setIsSaving(false);
        addToast({
          type: 'error',
          message: `Saving expertise settings failed: ${err instanceof Error ? err.message : 'sync engine unreachable'}`,
          dismissible: true,
        });
      },
    );
  };

  const setRepoExcludes = (repoId: string, value: string): void => {
    setExcludeInputs((current) => ({ ...current, [repoId]: value }));
  };

  return (
    <Section id="expertise" title="Expertise">
      <div className="space-y-6">
        <div>
          <h3 className="text-sm font-bold text-on-surface">
            Local expertise scoring
          </h3>
          <p className="text-xs text-on-surface-variant mt-1">
            Per-repo, opt-in indexing of your local git history. All scores stay
            in SQLite on this machine.
          </p>
        </div>

        <label className="block" htmlFor="expertise-alias-emails">
          <span className="block text-xs text-on-surface-variant mb-1">
            Alias emails
          </span>
          <Textarea
            id="expertise-alias-emails"
            value={aliasInput}
            rows={4}
            placeholder="you@company.com\n12345+you@users.noreply.github.com"
            onChange={(e) => setAliasInput(e.target.value)}
          />
          <span className="mt-1 block font-mono text-[10px] uppercase tracking-tight text-outline">
            One per line or comma-separated; counted as you for expertise
            matches.
          </span>
        </label>

        <label className="block max-w-xs" htmlFor="expertise-min-lines">
          <span className="block text-xs text-on-surface-variant mb-1">
            Minimum surviving lines
          </span>
          <Input
            id="expertise-min-lines"
            type="number"
            min={1}
            value={minLinesInput}
            onChange={(e) => setMinLinesInput(e.target.value)}
          />
        </label>

        <div className="space-y-3">
          <div>
            <h3 className="text-sm font-bold text-on-surface">
              Per-repo exclude globs
            </h3>
            <p className="text-xs text-on-surface-variant mt-1">
              Add generated or vendored paths to skip during indexing.
            </p>
          </div>
          {watchedRepos.length === 0 ? (
            <p className="font-mono text-[11px] text-outline uppercase tracking-widest">
              No watched repositories.
            </p>
          ) : (
            <div className="space-y-3">
              {watchedRepos.map((repo) => (
                <label
                  key={repo.id}
                  className="block"
                  htmlFor={`expertise-excludes-${repo.id}`}
                >
                  <span className="block text-xs text-on-surface-variant mb-1">
                    {repo.owner}/{repo.repo}
                  </span>
                  <Textarea
                    id={`expertise-excludes-${repo.id}`}
                    value={excludeInputs[repo.id] ?? ''}
                    rows={3}
                    placeholder="website/public/**\ngenerated/**"
                    onChange={(e) => setRepoExcludes(repo.id, e.target.value)}
                  />
                </label>
              ))}
            </div>
          )}
        </div>

        <div className="flex justify-end">
          <Button type="button" onClick={save} disabled={isSaving}>
            {isSaving ? 'Saving…' : 'Save expertise settings'}
          </Button>
        </div>
      </div>
    </Section>
  );
}
