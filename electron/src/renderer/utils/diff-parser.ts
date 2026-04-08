export interface DiffLine {
  type: 'context' | 'added' | 'removed' | 'hunk-header' | 'file-header';
  content: string;
  oldLineNumber?: number;
  newLineNumber?: number;
}

export interface DiffHunk {
  header: string;
  lines: DiffLine[];
}

export interface DiffSection {
  oldFile: string;
  newFile: string;
  hunks: DiffHunk[];
}

/**
 * Parse a multi-file unified diff (as produced by `git diff`) into an array of DiffSections.
 * Splits on `diff --git` boundaries and delegates each section to `parseDiff`.
 */
export function parseMultiFileDiff(diffText: string): DiffSection[] {
  if (!diffText || diffText.trim() === '') return [];

  // Split on "diff --git" boundaries
  const parts = diffText.split(/^(?=diff --git )/m);
  const sections: DiffSection[] = [];

  for (const part of parts) {
    const trimmed = part.trim();
    if (trimmed === '') continue;

    // Extract git header info for fallback file names
    const gitMatch = trimmed.match(/^diff --git a\/(.+) b\/(.+)/);

    // Strip the "diff --git a/... b/..." line and optional metadata lines
    const withoutGitHeader = trimmed
      .replace(/^diff --git [^\n]+\n?/, '')
      .replace(/^index [^\n]+\n?/m, '')
      .replace(/^(new|deleted) file mode [^\n]+\n?/gm, '')
      .replace(/^Binary files [^\n]+\n?/gm, '');

    if (withoutGitHeader.trim() === '') {
      // Binary or empty section — create a stub with names from git header
      if (gitMatch) {
        sections.push({
          oldFile: `a/${gitMatch[1]}`,
          newFile: `b/${gitMatch[2] ?? gitMatch[1]}`,
          hunks: [],
        });
      }
      continue;
    }

    const section = parseDiff(withoutGitHeader);

    // If parseDiff could not extract file names, fall back to git header
    if (!section.oldFile && !section.newFile && gitMatch) {
      sections.push({
        ...section,
        oldFile: `a/${gitMatch[1]}`,
        newFile: `b/${gitMatch[2] ?? gitMatch[1]}`,
      });
    } else {
      sections.push(section);
    }
  }

  return sections;
}

export function parseDiff(diffText: string): DiffSection {
  const lines = diffText.split('\n');
  const result: DiffSection = {
    oldFile: '',
    newFile: '',
    hunks: [],
  };

  let currentHunk: DiffHunk | null = null;
  let oldLineNum = 0;
  let newLineNum = 0;

  for (const line of lines) {
    if (line.startsWith('--- ')) {
      result.oldFile = line.slice(4);
    } else if (line.startsWith('+++ ')) {
      result.newFile = line.slice(4);
    } else if (line.startsWith('@@')) {
      const match = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
      if (match) {
        oldLineNum = parseInt(match[1] ?? '0', 10);
        newLineNum = parseInt(match[2] ?? '0', 10);
      }
      currentHunk = { header: line, lines: [] };
      result.hunks.push(currentHunk);
    } else if (currentHunk) {
      if (line.startsWith('\\')) {
        // Skip "\ No newline at end of file" markers
        continue;
      }

      let type: DiffLine['type'];
      let content: string;

      if (line.startsWith('-')) {
        type = 'removed';
        content = line.slice(1);
        const diffLine: DiffLine = { type, content, oldLineNumber: oldLineNum };
        currentHunk.lines.push(diffLine);
        oldLineNum++;
      } else if (line.startsWith('+')) {
        type = 'added';
        content = line.slice(1);
        const diffLine: DiffLine = { type, content, newLineNumber: newLineNum };
        currentHunk.lines.push(diffLine);
        newLineNum++;
      } else {
        type = 'context';
        content = line.startsWith(' ') ? line.slice(1) : line;
        const diffLine: DiffLine = {
          type,
          content,
          oldLineNumber: oldLineNum,
          newLineNumber: newLineNum,
        };
        currentHunk.lines.push(diffLine);
        oldLineNum++;
        newLineNum++;
      }
    }
  }

  return result;
}
