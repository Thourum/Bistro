import { performance } from 'node:perf_hooks';
import { parseMultiFileDiff } from '../src/utils/diff-parser.ts';

function generateDiff(files, changedLinesPerFile) {
  const chunks = [];
  for (let i = 0; i < files; i += 1) {
    const filePath = `src/generated/file_${String(i).padStart(5, '0')}.ts`;
    const lines = [
      `diff --git a/${filePath} b/${filePath}`,
      'index 0000000..1111111 100644',
      `--- a/${filePath}`,
      `+++ b/${filePath}`,
      `@@ -1,${changedLinesPerFile} +1,${changedLinesPerFile} @@`,
    ];
    for (let j = 0; j < changedLinesPerFile; j += 1) {
      lines.push(`-export const value_${j} = ${j};`);
      lines.push(`+export const value_${j} = ${j + 1};`);
    }
    chunks.push(lines.join('\n'));
  }
  return chunks.join('\n');
}

function measureParse(diff) {
  const start = performance.now();
  const sections = parseMultiFileDiff(diff);
  return { elapsed: performance.now() - start, sections: sections.length };
}

function percentile(sorted, pct) {
  return sorted[Math.ceil(sorted.length * pct) - 1] ?? sorted.at(-1) ?? 0;
}

const scenarios = [
  { name: '100 files x 20 changed lines', files: 100, lines: 20 },
  { name: '1000 files x 10 changed lines', files: 1000, lines: 10 },
  { name: '3000 files x 4 changed lines', files: 3000, lines: 4 },
  { name: '1 file x 10000 changed lines', files: 1, lines: 10000 },
];

for (const scenario of scenarios) {
  const diff = generateDiff(scenario.files, scenario.lines);
  parseMultiFileDiff(diff);

  const runs = [];
  let sections = 0;
  for (let i = 0; i < 7; i += 1) {
    const result = measureParse(diff);
    runs.push(result.elapsed);
    sections = result.sections;
  }

  runs.sort((a, b) => a - b);
  const median = percentile(runs, 0.5);
  const p95 = percentile(runs, 0.95);
  console.log(
    [
      `SUMMARY ${scenario.name}`,
      `sections=${sections}`,
      `bytes=${Buffer.byteLength(diff, 'utf8')}`,
      `lines=${diff.split('\n').length}`,
      `median=${median.toFixed(2)}ms`,
      `p95=${p95.toFixed(2)}ms`,
    ].join(' '),
  );
}
