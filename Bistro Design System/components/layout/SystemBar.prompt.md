The signature fixed bottom status bar — keyboard legend left, status chrome right.

```jsx
<SystemBar
  legend={[
    { key: 'J/K', label: 'Navigate' },
    { key: 'SPACE', label: 'Toggle fold' },
    { key: 'ENTER', label: 'View PR' },
  ]}
  status="System_Status: Operational"
  version="V.01"
  statusTone="success"
/>
```

Pin it to the bottom of a full-height flex column.
