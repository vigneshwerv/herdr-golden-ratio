# Behaviour

## The focused pane gets 61.8% of the tab

Not 61.8% of whatever split happens to contain it — 61.8% of the whole tab,
along each axis that separates it from the root.

This distinction matters because Herdr's layout is a binary tree, and splitting
the same direction twice **nests** rather than producing three siblings:

```
┌──────┬──────┬──────┐
│      │      │      │   root  ─split(right)─ A , inner
│  A   │  B   │  C   │   inner ─split(right)─ B , C
│      │      │      │
└──────┴──────┴──────┘
```

`B` is not a child of the root; it is a child of `inner`, which is itself only
half the tab. So every split between the root and the focused pane has to be
adjusted. If an ancestor is left alone, its ratio scales the result down —
giving `B` 61.8% of `inner` while `inner` stays at 50% leaves `B` at
0.618 × 0.5 = **31%** of the screen, which is not what anyone means by "golden
ratio".

Splits are grouped by axis, since `right` splits divide width and `down` splits
divide height and the two are independent. Where an axis is crossed *k* times,
each split on it gives the focused side `ratio^(1/k)`, so the product across
them is exactly `ratio`. Focusing `B` above sets both `right` splits to
√0.618 ≈ 0.786 of their space, and 0.786 × 0.786 = 0.618.

The result is that the focused pane reaches the same 61.8% no matter how deeply
it is nested:

| focused | width | height |
|---|---|---|
| A | 61.8% | 100% |
| B | 61.8% | 100% |
| C | 61.8% | 100% |

An axis the pane is *not* split on is left completely alone — a pane in a
column layout keeps its full height, and no split that does not lie between the
root and the focused pane is touched.

## Cases that do nothing, quietly

- A tab with a single pane (no split to adjust).
- A zoomed pane (it already fills the tab; resizing underneath it would surprise
  you on unzoom).
- A split already within `min_delta` of its target — checked per split, so the
  ones that do need moving still move.
- A pane that closed between the trigger and the resize.

None of these are errors, so they do not clutter `herdr plugin log list`.
