package main

import (
	"errors"
	"math"
	"testing"
)

func pane(id string) *Node {
	return &Node{Type: "pane", PaneID: id}
}

func split(dir string, ratio float64, first, second *Node) *Node {
	return &Node{Type: "split", Direction: dir, Ratio: ratio, First: first, Second: second}
}

func eq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func pathEq(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// applyPlan replays a plan onto a copy of the tree, so tests can assert on the
// geometry a user would actually see rather than on individual ratios.
func applyPlan(root *Node, steps []Resize) *Node {
	out := clone(root)
	for _, s := range steps {
		if n := nodeAt(out, s.Path); n != nil && n.Type == "split" {
			n.Ratio = s.Ratio
		}
	}
	return out
}

func clone(n *Node) *Node {
	if n == nil {
		return nil
	}
	c := *n
	c.First = clone(n.First)
	c.Second = clone(n.Second)
	return &c
}

// share returns the pane's fraction of the tab along one axis ("right" splits
// divide width, "down" splits divide height).
func share(n *Node, paneID, axis string) float64 {
	path, ok := findPane(n, paneID)
	if !ok {
		return 0
	}
	f := 1.0
	cur := n
	for _, second := range path {
		if cur.Direction == axis {
			if second {
				f *= 1 - cur.Ratio
			} else {
				f *= cur.Ratio
			}
		}
		if second {
			cur = cur.Second
		} else {
			cur = cur.First
		}
	}
	return f
}

func mustPlan(t *testing.T, root *Node, id string, ratio float64) []Resize {
	t.Helper()
	steps, err := ComputeGoldenRatio(root, id, ratio)
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

// A lone pane has no parent split to adjust.
func TestSinglePane(t *testing.T) {
	_, err := ComputeGoldenRatio(pane("p1"), "p1", Golden)
	if !errors.Is(err, ErrSinglePane) {
		t.Fatalf("want ErrSinglePane, got %v", err)
	}
}

// Focused pane on the First side: the ratio is used as-is.
func TestTwoPanesFocusFirst(t *testing.T) {
	root := split("right", 0.5, pane("p1"), pane("p2"))
	steps := mustPlan(t, root, "p1", Golden)
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	if len(steps[0].Path) != 0 {
		t.Errorf("want root path (empty), got %v", steps[0].Path)
	}
	if !eq(steps[0].Ratio, Golden) {
		t.Errorf("want ratio %v, got %v", Golden, steps[0].Ratio)
	}
}

// Focused pane on the Second side: the ratio must be complemented, because
// set_split_ratio always describes the First child's share.
func TestTwoPanesFocusSecond(t *testing.T) {
	root := split("right", 0.5, pane("p1"), pane("p2"))
	steps := mustPlan(t, root, "p2", Golden)
	if !eq(steps[0].Ratio, 1-Golden) {
		t.Errorf("want ratio %v, got %v", 1-Golden, steps[0].Ratio)
	}
	if !steps[0].FocusedIsSecond {
		t.Error("p2 is the Second child")
	}
}

// Regression: splitting right twice nests rather than making three siblings.
//
//	root(right){ A, inner(right){ B, C } }
//
// Adjusting only the immediate parent gave B 0.618 of `inner`, and `inner` was
// half the tab, so B ended up at 31% of the width instead of 61.8%.
func TestThreeColumnsEveryPaneReachesGoldenWidth(t *testing.T) {
	build := func() *Node {
		return split("right", 0.5,
			pane("A"),
			split("right", 0.5, pane("B"), pane("C")),
		)
	}
	for _, id := range []string{"A", "B", "C"} {
		root := build()
		got := share(applyPlan(root, mustPlan(t, root, id, Golden)), id, "right")
		if math.Abs(got-Golden) > 1e-9 {
			t.Errorf("pane %s: want %.4f of the tab width, got %.4f", id, Golden, got)
		}
	}
}

// The nested mixed-axis case: root(right){ A, inner(down){ B, C } }. A focused
// pane must reach the golden share on every axis that separates it from the
// root, and be left alone on axes it is not split on.
func TestNestedMixedAxes(t *testing.T) {
	build := func() *Node {
		return split("right", 0.5,
			pane("A"),
			split("down", 0.5, pane("B"), pane("C")),
		)
	}
	root := build()
	after := applyPlan(root, mustPlan(t, root, "C", Golden))
	if w := share(after, "C", "right"); math.Abs(w-Golden) > 1e-9 {
		t.Errorf("C width: want %.4f, got %.4f", Golden, w)
	}
	if h := share(after, "C", "down"); math.Abs(h-Golden) > 1e-9 {
		t.Errorf("C height: want %.4f, got %.4f", Golden, h)
	}

	// A is only separated from the root by a "right" split, so its height is
	// the full tab and only its width should change.
	root = build()
	after = applyPlan(root, mustPlan(t, root, "A", Golden))
	if w := share(after, "A", "right"); math.Abs(w-Golden) > 1e-9 {
		t.Errorf("A width: want %.4f, got %.4f", Golden, w)
	}
	if h := share(after, "A", "down"); math.Abs(h-1.0) > 1e-9 {
		t.Errorf("A height: want full tab, got %.4f", h)
	}
}

// Four panes in a row: three "right" splits on the path to the last pane, each
// taking the cube root, still multiplying out to the golden share.
func TestFourColumnsDeepNesting(t *testing.T) {
	build := func() *Node {
		return split("right", 0.5,
			pane("A"),
			split("right", 0.5,
				pane("B"),
				split("right", 0.5, pane("C"), pane("D")),
			),
		)
	}
	for _, id := range []string{"A", "B", "C", "D"} {
		root := build()
		steps := mustPlan(t, root, id, Golden)
		got := share(applyPlan(root, steps), id, "right")
		if math.Abs(got-Golden) > 1e-9 {
			t.Errorf("pane %s: want %.4f of the tab width, got %.4f", id, Golden, got)
		}
	}
}

// Every split between the root and the pane is adjusted, and each step
// addresses a distinct split.
func TestPlanCoversEveryAncestorSplit(t *testing.T) {
	root := split("right", 0.5,
		pane("A"),
		split("down", 0.5,
			pane("B"),
			split("right", 0.5, pane("C"), pane("D")),
		),
	)
	steps := mustPlan(t, root, "D", Golden)
	if len(steps) != 3 {
		t.Fatalf("want 3 steps for a pane 3 levels deep, got %d", len(steps))
	}
	want := [][]bool{{}, {true}, {true, true}}
	for i, w := range want {
		if !pathEq(steps[i].Path, w) {
			t.Errorf("step %d: want path %v, got %v", i, w, steps[i].Path)
		}
	}
	// Root first, so the layout does not visibly bounce.
	if len(steps[0].Path) != 0 {
		t.Error("first step should be the root split")
	}
}

// A pane that is a direct child of the root still behaves as it did before:
// a single split set to the plain ratio.
func TestShallowPaneUnchangedBehaviour(t *testing.T) {
	root := split("right", 0.5,
		pane("A"),
		split("down", 0.5, pane("B"), pane("C")),
	)
	steps := mustPlan(t, root, "A", Golden)
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	if !eq(steps[0].Ratio, Golden) {
		t.Errorf("want ratio %v, got %v", Golden, steps[0].Ratio)
	}
}

func TestPaneNotFound(t *testing.T) {
	root := split("right", 0.5, pane("p1"), pane("p2"))
	if _, err := ComputeGoldenRatio(root, "nope", Golden); !errors.Is(err, ErrPaneNotFound) {
		t.Fatalf("want ErrPaneNotFound, got %v", err)
	}
}

// Current is reported so callers can skip a redundant write.
func TestReportsCurrentRatio(t *testing.T) {
	root := split("right", 0.42, pane("p1"), pane("p2"))
	steps := mustPlan(t, root, "p1", Golden)
	if !eq(steps[0].Current, 0.42) {
		t.Errorf("want current 0.42, got %v", steps[0].Current)
	}
}

// An extreme configured ratio is clamped so a pane never collapses entirely.
func TestRatioClamped(t *testing.T) {
	root := split("right", 0.5, pane("p1"), pane("p2"))
	steps := mustPlan(t, root, "p2", 0.999)
	if steps[0].Ratio < minRatio {
		t.Errorf("ratio %v below clamp %v", steps[0].Ratio, minRatio)
	}
}

// Re-applying a plan to an already-golden layout is a fixed point.
func TestIdempotent(t *testing.T) {
	root := split("right", 0.5,
		pane("A"),
		split("right", 0.5, pane("B"), pane("C")),
	)
	once := applyPlan(root, mustPlan(t, root, "B", Golden))
	twice := applyPlan(once, mustPlan(t, once, "B", Golden))
	if got := share(twice, "B", "right"); math.Abs(got-Golden) > 1e-9 {
		t.Errorf("want stable %.4f, got %.4f", Golden, got)
	}
}

func TestInvalidRatioRejected(t *testing.T) {
	root := split("right", 0.5, pane("p1"), pane("p2"))
	for _, r := range []float64{0, 1, -0.5, 1.5} {
		if _, err := ComputeGoldenRatio(root, "p1", r); err == nil {
			t.Errorf("ratio %v should be rejected", r)
		}
	}
}

// Golden must be the reciprocal of phi, i.e. satisfy x^2 + x = 1.
func TestGoldenConstant(t *testing.T) {
	if !eq(Golden*Golden+Golden, 1) {
		t.Errorf("Golden %v is not 1/phi", Golden)
	}
}
