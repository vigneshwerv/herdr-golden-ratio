# Behaviour

## Only the immediate parent split is changed

Given a tab like this, with **C** focused:

```
┌─────────────┬─────────────┐
│             │      B      │
│      A      ├─────────────┤   root  ─split(right)─ A , inner
│             │      C      │   inner ─split(down)──  B , C
└─────────────┴─────────────┘
```

C gets 61.8% of `inner` (the vertical space it shares with B). The root split
stays where it is, so the A-versus-BC balance is untouched.

Propagating the ratio up the tree was deliberately rejected: it compounds. A
pane three levels deep would end up with 0.618³ ≈ 24% of the tab — *shrinking*
the more nested it is, the opposite of the intent — and it would disturb splits
containing panes you never focused. Adjusting one split is also trivial to undo
by hand.

## Cases that do nothing, quietly

- A tab with a single pane (no split to adjust).
- A zoomed pane (it already fills the tab; resizing underneath it would surprise
  you on unzoom).
- A split already within `min_delta` of the target.
- A pane that closed between the trigger and the resize.

None of these are errors, so they do not clutter `herdr plugin log list`.
