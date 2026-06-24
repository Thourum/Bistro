Flat colored square encoding CI / sync / review state — Bistro's status atom. Never round.

```jsx
<StatusDot status="success" />
<StatusDot status="failure" size={8} />
<StatusDot status="running" />   {/* pulses */}
```

States: `success` (green), `failure` (red), `running`/`pending` (amber, pulsing), `unknown` (gray).
