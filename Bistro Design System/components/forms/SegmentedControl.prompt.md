Boxed segmented filter — the inbox lane/state switcher. Active segment fills blue. Options can carry counts.

```jsx
<SegmentedControl
  value={filter}
  onChange={setFilter}
  options={[
    { value: 'needs', label: 'Needs Action', count: 5 },
    { value: 'open', label: 'Open', count: 12 },
    { value: 'draft', label: 'Draft', count: 2 },
    { value: 'all', label: 'All' },
  ]}
/>
```
