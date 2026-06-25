Square, mono, keyboard-forward button — use for every click target; pair primary actions with their `keyHint`.

```jsx
<Button variant="primary" keyHint="A">Approve</Button>
<Button variant="outline" icon="sync">Sync now</Button>
<Button variant="ghost">Cancel</Button>
<Button variant="danger" keyHint="R">Request changes</Button>
```

Variants: `primary` (Bistro blue), `outline` (workhorse secondary), `ghost` (tertiary), `danger` (destructive, fills red on hover). Sizes: `sm` / `md` / `lg`. Labels render UPPERCASE in JetBrains Mono — pass natural-case text.
