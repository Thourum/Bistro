import type { BindingDefinition } from './keybindings';

const DETAIL = { routes: ['detail'] } as const;
const DIFF = { routes: ['diff'] } as const;
const COMMITS = { routes: ['commits'] } as const;
const COMMIT = { routes: ['commit'] } as const;
const COMMIT_VIEWS = { routes: ['commits', 'commit'] } as const;

export const COMMIT_BINDINGS: readonly BindingDefinition[] = [
  {
    actionId: 'detail.commits',
    description: 'Open commits',
    activation: DETAIL,
  },
  { actionId: 'diff.commits', description: 'Open commits', activation: DIFF },
  { actionId: 'commits.down', description: 'Next commit', activation: COMMITS },
  {
    actionId: 'commits.up',
    description: 'Previous commit',
    activation: COMMITS,
  },
  { actionId: 'commits.open', description: 'Open commit', activation: COMMITS },
  { actionId: 'commit.next', description: 'Next commit', activation: COMMIT },
  {
    actionId: 'commit.prev',
    description: 'Previous commit',
    activation: COMMIT,
  },
  {
    actionId: 'commit.toggleViewed',
    description: 'Toggle commit viewed',
    activation: COMMIT_VIEWS,
  },
  {
    actionId: 'commit.viewedAndNext',
    description: 'Mark viewed and next commit',
    activation: COMMIT,
  },
];
