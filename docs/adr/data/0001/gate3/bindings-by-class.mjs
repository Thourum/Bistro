import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';

const DIRECTORY = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(DIRECTORY, '../../../../..');
/** Parse the source-only binding inventory; importing this module has no artifact side effects. */
export function parseInventory(root = ROOT) {
const readSource = (name) => readFileSync(join(root, 'electron/src/renderer', name), 'utf8');
const keymaps = readSource('utils/keymaps.ts');
const catalog = readSource('utils/keybindings.ts');
const countConstant = (source, name) => Number(source.match(new RegExp(`const ${name} = (\\d+);`))?.[1]);
const jumpCount = countConstant(keymaps, 'LIST_JUMP_COUNT');
const laneCount = countConstant(keymaps, 'LANE_COUNT');
assert.equal(jumpCount, countConstant(catalog, 'LIST_JUMP_COUNT'));
assert.equal(laneCount, countConstant(catalog, 'LANE_COUNT'));

function parseTable(name) {
  const body = keymaps.match(new RegExp(`const ${name}: KeyTable = \\{([\\s\\S]*?)\\n\\};`))?.[1];
  assert.ok(body, `Missing table ${name}`);
  return Object.fromEntries([...body.matchAll(/'([^']+)':\s*('[^']*'|\[[^\]]*\])/g)].map((match) => {
    const tokens = [...match[2].matchAll(/'([^']*)'/g)].map((token) => token[1]);
    return [match[1], match[2].startsWith('[') ? tokens : tokens[0]];
  }));
}

const providers = [...readSource('providers/index.ts').matchAll(/^import \{ (\w+) \} from '\.\/(\w+)';/gm)]
  .map((match) => readSource(`providers/${match[2]}.ts`))
  .filter((source) => /isAvailable:\s*true/.test(source))
  .map((source) => ({ id: source.match(/id:\s*'([^']+)'/)[1], key: source.match(/key:\s*'([^']+)'/)[1] }));
const generatedEntries = (prefix, count, format) => Object.fromEntries(
  Array.from({ length: count }, (_, index) => [`${prefix}${index + 1}`, format(index + 1)]));
const availableProviderKeys = Object.fromEntries(providers.map((provider) => [`login.provider.${provider.id}`, provider.key]));
const shared = { ...parseTable('SHARED_KEYS'), ...availableProviderKeys };
const catalogIds = new Set([...catalog.matchAll(/actionId:\s*'([^']+)'/g)].map((match) => match[1]));
for (const id of Object.keys({ ...availableProviderKeys,
  ...generatedEntries('list.jumpTo', jumpCount, String), ...generatedEntries('inbox.lane', laneCount, String) })) catalogIds.add(id);
const tables = {
  vim: { ...shared, ...generatedEntries('list.jumpTo', jumpCount, String),
    ...generatedEntries('inbox.lane', laneCount, (digit) => ['g', String(digit)]), ...parseTable('VIM_KEYS') },
  emacs: { ...shared, ...generatedEntries('list.jumpTo', jumpCount, (digit) => `Alt+${digit}`),
    ...generatedEntries('inbox.lane', laneCount, (digit) => ['Control+C', String(digit)]), ...parseTable('EMACS_KEYS') },
  standard: { ...shared, ...generatedEntries('list.jumpTo', jumpCount, String),
    ...generatedEntries('inbox.lane', laneCount, (digit) => `Mod+${digit}`), ...parseTable('STANDARD_KEYS') },
};
return { tables, catalogIds: [...catalogIds] };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
const { tables, catalogIds: ids } = parseInventory();
const catalogIds = new Set(ids);
const predicates = {
  'plain, no modifier (single step)': (steps, isSequence) => !isSequence && !/\+/.test(steps[0]),
  'Cmd / Mod on macOS (any step)': (steps) => steps.some((step) => /(?:Meta|Mod)\+/.test(step)),
  'Ctrl (any step)': (steps) => steps.some((step) => /Control\+/.test(step)),
  'Option / Alt (any step)': (steps) => steps.some((step) => /Alt\+/.test(step)),
  'multi-key sequences': (_steps, isSequence) => isSequence,
  'Shift-only single step (additional coverage)': (steps, isSequence) => !isSequence && /^Shift\+/.test(steps[0]),
};
const classified = Object.fromEntries(Object.entries(tables).map(([style, table]) => {
  assert.ok(Object.keys(table).every((id) => catalogIds.has(id)), 'Key table has action absent from catalog');
  const entries = Object.entries(table).map(([actionId, keys]) => ({ actionId, keys,
    steps: Array.isArray(keys) ? keys : [keys], isSequence: Array.isArray(keys) }));
  return [style, { total: entries.length, unbound: [...catalogIds].filter((id) => !(id in table)),
    classes: Object.fromEntries(Object.entries(predicates).map(([name, predicate]) => [name,
      entries.filter((entry) => predicate(entry.steps, entry.isSequence)).map(({ actionId, keys }) => ({ actionId, keys }))])) }];
}));
assert.ok(classified.emacs.classes['Ctrl (any step)'].some((entry) => entry.actionId === 'detail.approve'));
assert.ok(classified.vim.classes['multi-key sequences'].some((entry) => entry.actionId === 'detail.approve'));
assert.ok(classified.standard.classes['Cmd / Mod on macOS (any step)'].some((entry) => entry.actionId === 'detail.undoDismiss'));
assert.ok(classified.standard.classes['multi-key sequences'].some((entry) => entry.actionId === 'detail.approve'));
const tableRows = Object.keys(predicates).map((name) => `| ${name} | ${classified.vim.classes[name].length} | ${classified.emacs.classes[name].length} | ${classified.standard.classes[name].length} |`);
const totals = `| All bound actionIds | ${classified.vim.total} | ${classified.emacs.total} | ${classified.standard.total} |`;
writeFileSync(join(DIRECTORY, 'bindings-by-class.json'), JSON.stringify(classified, null, 2) + '\n');
writeFileSync(join(DIRECTORY, 'bindings-by-class.md'), `# Binding inventory (no key injection)\n\nReproduce: \`node docs/adr/data/0001/gate3/bindings-by-class.mjs\` from the repository root. Reads files as text; imports/evaluates no application or dependency code. Fixed source-table grammar is asserted; generated provider, PR-jump and lane entries are expanded using source constants. Counts are actionIds, not distinct chords or active-route counts. User overrides are absent.\n\n| Class | vim | emacs | standard |\n|---|---:|---:|---:|\n${tableRows.join('\n')}\n${totals}\n\nClasses overlap: Ctrl+Alt belongs to both Ctrl and Option; every modified sequence belongs to its modifier class and sequences. The plain row excludes Shift-only and sequences; both are still covered by S6. Option US and DE/AZERTY/Dvorak use the same Option inventory. Every bound actionId is a focus/IME test candidate, including expected non-firing cells. \`global.quit.win\` (Alt+F4) is in the source table for all styles but platform-disabled on macOS: subtract one from Option and total for the macOS-active inventory. Hidden arrow aliases are counted because they remain registered.\n\nInside comment textarea: normal global bindings/sequences use ignoreInputs; Escape (plus emacs C-g for active overlays) can bypass this. Local \`TEXTAREA_SUBMIT_HOTKEY = Mod+Enter\` is one additional binding in every style (function accepts Cmd **or Ctrl**+Enter), not in ACTION_CATALOG. Sandbox-only Shift+Space is not a comment-textarea binding. IME-active is a cross-cutting context, not a separate set of chords.\n\nFull actionId/chord membership: [bindings-by-class.json](bindings-by-class.json).\n`);
console.log(JSON.stringify(Object.fromEntries(Object.entries(classified).map(([style, inventory]) => [style,
  { total: inventory.total, unbound: inventory.unbound, counts: Object.fromEntries(Object.entries(inventory.classes).map(([name, entries]) => [name, entries.length])) }])), null, 2));
}
