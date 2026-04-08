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
