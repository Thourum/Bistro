One row of a unified diff — gutters + marker + mono code. Compose many into a file diff.

```jsx
<DiffLine oldNo={42} newNo={42}>  log::info!("Starting Kernel…");</DiffLine>
<DiffLine newNo={43} type="remove">  self.status = EngineStatus::Initializing;</DiffLine>
<DiffLine oldNo={43} type="add">  self.set_state(EngineStatus::Initializing)?;</DiffLine>
<DiffLine oldNo={44} type="add" focused>  self.verify_integrity_hash()?;</DiffLine>
```
