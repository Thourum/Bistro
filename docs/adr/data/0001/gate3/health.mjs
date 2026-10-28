import { execFileSync, execFile } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const directory = dirname(fileURLToPath(import.meta.url));
const since = Date.parse('2025-10-27T00:00:00Z');
const until = Date.parse('2026-10-27T00:00:00Z');
const concurrency = 4;
const millisecondsPerDay = 86_400_000;
const maintainerAssociations = new Set(['OWNER', 'MEMBER', 'COLLABORATOR']);
const run = promisify(execFile);
const read = (name) => JSON.parse(readFileSync(join(directory, name), 'utf8'));
const flatten = (name) => read(name).flat();
const save = (name, text) => writeFileSync(join(directory, name), text);
const issues = () => read('health-issues.json').items;

async function collectComments() {
  const selected = issues();
  for (let index = 0; index < selected.length; index += concurrency) {
    await Promise.all(selected.slice(index, index + concurrency).map(async (issue) => {
      const { stdout } = await run('gh', ['api', '--paginate', '--slurp',
        `repos/wailsapp/wails/issues/${issue.number}/comments?per_page=100`],
      { maxBuffer: 32 * 1024 * 1024 });
      save(`health-comments-${issue.number}.json`, stdout);
    }));
  }
}

function median(values) {
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2;
}

function summarize() {
  const releases = flatten('health-releases.json').filter((release) =>
    !release.draft && Date.parse(release.published_at) >= since && Date.parse(release.published_at) < until);
  const commits = flatten('health-commits.json');
  const isApiBot = (commit) => commit.author?.type === 'Bot' ||
    /\[bot\]|dependabot|github-actions/i.test(`${commit.author?.login ?? ''} ${commit.commit.author.name} ${commit.commit.author.email}`);
  // Some automation uses GitHub User accounts; exact author identities avoid excluding names such as Botterman.
  const automationNames = new Set(['taliesin-ai', 'Multica Bot', 'Wails Doc Translator',
    'Engineer - Linux (Multica Agent)', 'Wails Documentation Agent', 'GitHub Action']);
  const isBot = (commit) => isApiBot(commit) || automationNames.has(commit.commit.author.name);
  const apiNonBots = commits.filter((commit) => !isApiBot(commit));
  const counts = new Map();
  const humans = commits.filter((commit) => !isBot(commit));
  for (const commit of humans) {
    const author = commit.author?.login ?? `${commit.commit.author.name} <${commit.commit.author.email}>`;
    counts.set(author, (counts.get(author) ?? 0) + 1);
  }
  const authors = [...counts].map(([author, count]) => ({ author, count })).sort((left, right) => right.count - left.count);
  const responseRows = issues().map((issue) => {
    const first = flatten(`health-comments-${issue.number}.json`).filter((comment) =>
      maintainerAssociations.has(comment.author_association) && comment.user.login !== issue.user.login &&
      Date.parse(comment.created_at) < until).sort((left, right) => Date.parse(left.created_at) - Date.parse(right.created_at))[0];
    return { number: issue.number, created: issue.created_at, author: issue.user.login,
      firstMaintainer: first?.user.login ?? null, firstResponse: first?.created_at ?? null,
      days: first ? (Date.parse(first.created_at) - Date.parse(issue.created_at)) / millisecondsPerDay : null,
      censoredAgeDays: first ? null : (until - Date.parse(issue.created_at)) / millisecondsPerDay };
  });
  const responded = responseRows.filter((row) => row.days !== null).map((row) => row.days);
  const conservativeMedian = median(responseRows.map((row) => row.days ?? Infinity));
  return {
    interval: '[2025-10-27T00:00:00Z, 2026-10-27T00:00:00Z)',
    pin: { tag: 'v3.0.0', sha: '7b11001b92b712210b27d8cb53430e83f67ad8e4', developmentBranch: 'master', developmentSha: 'c40713d9523ec24a90aac28aa9cf9faac2ab384f' },
    releases: { allCount: releases.length, v3Count: releases.filter((release) => /^v3\./.test(release.tag_name)).length,
      dates: releases.map((release) => ({ tag: release.tag_name, date: release.published_at, prerelease: release.prerelease })) },
    commits: { total: commits.length, bots: commits.length - humans.length, nonBot: humans.length,
      definition: 'Commit author login (not committer or co-author); exclude API Bot accounts, [bot]/dependabot/github-actions identities, and exact automation author names listed here. AI-assisted human identities are not generally detectable.',
      automationNames: [...automationNames],
      apiIdentityOnly: { nonBot: apiNonBots.length, topAuthorSharePercent: apiNonBots.filter((commit) => commit.author?.login === authors[0].author).length / apiNonBots.length * 100 },
      topAuthorSharePercent: authors[0].count / humans.length * 100, authors },
    responses: { issueCount: responseRows.length, responded: responded.length, unresponded: responseRows.length - responded.length,
      definition: 'First issue comment by OWNER/MEMBER/COLLABORATOR other than the reporter, before cutoff; PRs excluded. Unanswered issues right-censored; conservative median assigns Infinity, respondent-only median reported separately. No closure/reaction counted as a response.',
      conservativeMedianDays: Number.isFinite(conservativeMedian) ? conservativeMedian : 'Infinity (not identifiable with >=50 unanswered)',
      respondentOnlyMedianDays: median(responded),
      lowerBoundMedianDays: median(responseRows.map((row) => row.days ?? row.censoredAgeDays)),
      rows: responseRows }
  };
}

function searchIssues() {
  const queries = [
    ['keyboard', 'keyboard'], ['keydown', 'keydown'], ['shortcuts', 'shortcut'],
    ['cmd', 'Cmd'], ['accelerator', 'accelerator'], ['ime', 'IME'],
    ['dead-key', '"dead key"'], ['layout', '"keyboard layout"'],
    ['perform-key-equivalent', 'performKeyEquivalent'], ['webkitgtk', 'WebKitGTK'],
    ['webkit-performance', 'webkit performance'], ['mac-keyboard', 'macOS keyboard'],
  ];
  for (const [name, term] of queries) {
    const stdout = execFileSync('gh', ['api', '--method', 'GET', '--paginate', '--slurp', 'search/issues',
      '-f', `q=repo:wailsapp/wails is:issue created:<2026-10-27 ${term}`, '-f', 'per_page=100'],
    { encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 });
    save(`issues-${name}.json`, stdout);
  }
}

async function collectIssueDetails() {
  const selected = [6033, 6056, 4061, 3722, 2604, 6170, 6249, 4918,
    5527, 5631, 4957, 4985, 5459, 5522, 6219, 4794, 6121];
  for (let index = 0; index < selected.length; index += concurrency) {
    await Promise.all(selected.slice(index, index + concurrency).map(async (number) => {
      const { stdout: issue } = await run('gh', ['api', `repos/wailsapp/wails/issues/${number}`]);
      save(`issue-${number}.json`, issue);
      const { stdout: comments } = await run('gh', ['api', '--paginate', '--slurp',
        `repos/wailsapp/wails/issues/${number}/comments?per_page=100`], { maxBuffer: 32 * 1024 * 1024 });
      save(`issue-${number}-comments.json`, comments);
    }));
  }
}

const actions = { collect: collectComments, details: collectIssueDetails,
  summarize: () => console.log(JSON.stringify(summarize(), null, 2)), search: searchIssues };
const action = actions[process.argv[2]];
if (!action) { console.error('Expected collect, details, summarize, or search'); process.exitCode = 1; }
else Promise.resolve(action()).then(() => {}, (error) => { console.error(error.message); process.exitCode = 1; });
