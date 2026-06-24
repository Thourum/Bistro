The primary logo lockup — mark + Geist wordmark, locked together.

```jsx
<BistroLockup size={32} />                          {/* horizontal, for headers/bars */}
<BistroLockup size={48} orientation="stacked" />    {/* centered splash */}
<BistroLockup size={28} variant="inverse" />        {/* on dark */}
```

The mark scales off `size` (the wordmark cap height) so the lockup stays proportional.
