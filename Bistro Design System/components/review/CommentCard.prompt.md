Inline review comment or composer — square avatar, uppercase mono handle, state tag, body, action row.

```jsx
<CommentCard author="J_Doe System Arch" tag="ACTIVE_DRAFT"
  actions={<><Button variant="ghost" size="sm">Cancel</Button><Button variant="primary" size="sm">Comment</Button></>}>
  Add a null check before init — tokens can be undefined here.
</CommentCard>
```

Omit `children` to show the empty composer placeholder.
