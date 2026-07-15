import {
  type FileDiffMetadata,
  type Hunk,
  parsePatchFiles,
  processFile,
} from '@pierre/diffs';

export interface DiffLine {
  type: 'context' | 'added' | 'removed';
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

interface DiffFileNames {
  oldFile: string;
  newFile: string;
}

/**
 * Parse a multi-file unified diff (as produced by `git diff`) into an array of DiffSections.
 * Uses @pierre/diffs for patch parsing and adapts its metadata to the legacy UI shape.
 */
export function parseMultiFileDiff(diffText: string): DiffSection[] {
  if (!diffText || diffText.trim() === '') return [];

  const fallbackNames = extractGitFileNames(diffText);
  return parsePatchFiles(diffText)
    .flatMap((patch) => patch.files)
    .map((file, index) => mapFileToSection(file, fallbackNames[index]));
}

export function parseDiff(diffText: string): DiffSection {
  if (!diffText || diffText.trim() === '') {
    return { oldFile: '', newFile: '', hunks: [] };
  }

  const parsedFile = processFile(diffText);
  if (!parsedFile) {
    return { oldFile: '', newFile: '', hunks: [] };
  }

  return mapFileToSection(parsedFile, extractUnifiedFileNames(diffText));
}

function mapFileToSection(
  file: FileDiffMetadata,
  fallbackNames?: DiffFileNames,
): DiffSection {
  return {
    oldFile: fallbackNames?.oldFile ?? getOldFileName(file),
    newFile: fallbackNames?.newFile ?? getNewFileName(file),
    hunks: file.hunks.map((hunk) => mapHunk(hunk, file)),
  };
}

function mapHunk(hunk: Hunk, file: FileDiffMetadata): DiffHunk {
  let oldLineNumber = hunk.deletionStart;
  let newLineNumber = hunk.additionStart;

  const lines = hunk.hunkContent.flatMap((content): DiffLine[] => {
    if (content.type === 'context') {
      const contextLines = range(content.lines).map((offset) => ({
        type: 'context' as const,
        content: normalizeLineContent(
          file.deletionLines[content.deletionLineIndex + offset] ?? '',
        ),
        oldLineNumber: oldLineNumber + offset,
        newLineNumber: newLineNumber + offset,
      }));

      oldLineNumber += content.lines;
      newLineNumber += content.lines;

      return contextLines;
    }

    const removedLines = range(content.deletions).map((offset) => ({
      type: 'removed' as const,
      content: normalizeLineContent(
        file.deletionLines[content.deletionLineIndex + offset] ?? '',
      ),
      oldLineNumber: oldLineNumber + offset,
    }));
    const addedLines = range(content.additions).map((offset) => ({
      type: 'added' as const,
      content: normalizeLineContent(
        file.additionLines[content.additionLineIndex + offset] ?? '',
      ),
      newLineNumber: newLineNumber + offset,
    }));

    oldLineNumber += content.deletions;
    newLineNumber += content.additions;

    return [...removedLines, ...addedLines];
  });

  return {
    header: normalizeHunkHeader(hunk.hunkSpecs),
    lines,
  };
}

function getOldFileName(file: FileDiffMetadata): string {
  if (file.type === 'new') return '/dev/null';
  return addGitPrefix('a', file.prevName ?? file.name);
}

function getNewFileName(file: FileDiffMetadata): string {
  if (file.type === 'deleted') return '/dev/null';
  return addGitPrefix('b', file.name);
}

function extractGitFileNames(diffText: string): DiffFileNames[] {
  return diffText
    .split(/^(?=diff --git )/m)
    .map((section) => section.trim())
    .filter((section) => section !== '')
    .map(
      (section) =>
        extractUnifiedFileNames(section) ?? extractGitHeaderNames(section),
    )
    .filter((names): names is DiffFileNames => names !== undefined);
}

function extractUnifiedFileNames(diffText: string): DiffFileNames | undefined {
  const oldFile = diffText.match(/^---\s+(.+)$/m)?.[1]?.trim();
  const newFile = diffText.match(/^\+\+\+\s+(.+)$/m)?.[1]?.trim();

  if (!oldFile && !newFile) return undefined;

  return {
    oldFile: oldFile ? unquotePath(oldFile) : '',
    newFile: newFile ? unquotePath(newFile) : '',
  };
}

function extractGitHeaderNames(diffText: string): DiffFileNames | undefined {
  const headerLine = diffText.match(/^diff --git (.+)$/m)?.[1];
  if (!headerLine) return undefined;

  const quotedMatch = headerLine.match(/^"a\/(.+)" "b\/(.+)"$/);
  if (quotedMatch) {
    return {
      oldFile: `a/${quotedMatch[1]}`,
      newFile: `b/${quotedMatch[2]}`,
    };
  }

  const lastNewPathPrefix = headerLine.lastIndexOf(' b/');
  if (lastNewPathPrefix === -1 || !headerLine.startsWith('a/')) {
    return undefined;
  }

  return {
    oldFile: headerLine.slice(0, lastNewPathPrefix),
    newFile: headerLine.slice(lastNewPathPrefix + 1),
  };
}

function addGitPrefix(prefix: 'a' | 'b', fileName: string): string {
  if (fileName === '' || fileName === '/dev/null') return fileName;
  if (fileName.startsWith('a/') || fileName.startsWith('b/')) return fileName;
  return `${prefix}/${fileName}`;
}

function unquotePath(path: string): string {
  if (path.length >= 2 && path.startsWith('"') && path.endsWith('"')) {
    return path.slice(1, -1);
  }
  return path;
}

function normalizeHunkHeader(header: string | undefined): string {
  return header?.replace(/[\r\n]+$/, '') ?? '';
}

function normalizeLineContent(line: string): string {
  return line.replace(/[\r\n]+$/, '');
}

function range(length: number): number[] {
  return Array.from({ length }, (_, index) => index);
}
