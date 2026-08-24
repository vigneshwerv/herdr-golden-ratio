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
	got, err := ComputeGoldenRatio(root, "p1", Golden)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Path) != 0 {
		t.Errorf("want root path (empty), got %v", got.Path)
	}
	if !eq(got.Ratio, Golden) {
		t.Errorf("want ratio %v, got %v", Golden, got.Ratio)
	}
	if got.FocusedIsSecond {
		t.Error("p1 is the First child")
	}
}

// Focused pane on the Second side: the ratio must be complemented, because
// set_split_ratio always describes the First child's share.
func TestTwoPanesFocusSecond(t *testing.T) {
	root := split("right", 0.5, pane("p1"), pane("p2"))
	got, err := ComputeGoldenRatio(root, "p2", Golden)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(got.Ratio, 1-Golden) {
		t.Errorf("want ratio %v, got %v", 1-Golden, got.Ratio)
	}
	if !got.FocusedIsSecond {
		t.Error("p2 is the Second child")
	}
}

// This mirrors the live three-pane tab used to verify the API by hand:
// root split right { p2 | split down { p3 / p4 } }. Focusing p4 must target the
// inner split at path [true] with ratio 1-0.618 == 0.382.
func TestNestedMatchesVerifiedLiveLayout(t *testing.T) {
	root := split("right", 0.5,
		pane("w2D:p2"),
		split("down", 0.5, pane("w2D:p3"), pane("w2D:p4")),
	)
	got, err := ComputeGoldenRatio(root, "w2D:p4", Golden)
	if err != nil {
		t.Fatal(err)
	}
	if !pathEq(got.Path, []bool{true}) {
		t.Errorf("want path [true], got %v", got.Path)
	}
	if !eq(got.Ratio, 1-Golden) {
		t.Errorf("want ratio %v, got %v", 1-Golden, got.Ratio)
	}
	if got.Direction != "down" {
		t.Errorf("want parent direction down, got %q", got.Direction)
	}
}

// The sibling of a nested split: p3 is First of the inner split.
func TestNestedFocusInnerFirst(t *testing.T) {
	root := split("right", 0.5,
		pane("p2"),
		split("down", 0.5, pane("p3"), pane("p4")),
	)
	got, err := ComputeGoldenRatio(root, "p3", Golden)
	if err != nil {
		t.Fatal(err)
	}
	if !pathEq(got.Path, []bool{true}) {
		t.Errorf("want path [true], got %v", got.Path)
	}
	if !eq(got.Ratio, Golden) {
		t.Errorf("want ratio %v, got %v", Golden, got.Ratio)
	}
}

// Focusing a pane that is itself a direct child of the root, while its sibling
// is a subtree, must adjust the root split and leave the subtree alone.
func TestNestedFocusOuterPaneTargetsRoot(t *testing.T) {
	root := split("right", 0.5,
		pane("p2"),
		split("down", 0.5, pane("p3"), pane("p4")),
	)
	got, err := ComputeGoldenRatio(root, "p2", Golden)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Path) != 0 {
		t.Errorf("want root path (empty), got %v", got.Path)
	}
	if !eq(got.Ratio, Golden) {
		t.Errorf("want ratio %v, got %v", Golden, got.Ratio)
	}
}

// Immediate-parent-only: a deeply nested pane still gets 0.618 of its own
// parent split, never a compounded 0.618^depth, and the path addresses only
// that one split.
func TestDeepNestingUsesImmediateParentOnly(t *testing.T) {
	deep := split("right", 0.5,
		pane("a"),
		split("down", 0.5,
			pane("b"),
			split("right", 0.5, pane("c"), pane("d")),
		),
	)
	got, err := ComputeGoldenRatio(deep, "d", Golden)
	if err != nil {
		t.Fatal(err)
	}
	if !pathEq(got.Path, []bool{true, true}) {
		t.Errorf("want path [true true], got %v", got.Path)
	}
	if !eq(got.Ratio, 1-Golden) {
		t.Errorf("want ratio %v (not compounded), got %v", 1-Golden, got.Ratio)
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
	got, err := ComputeGoldenRatio(root, "p1", Golden)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(got.Current, 0.42) {
		t.Errorf("want current 0.42, got %v", got.Current)
	}
}

// An extreme configured ratio is clamped so a pane never collapses entirely.
func TestRatioClamped(t *testing.T) {
	root := split("right", 0.5, pane("p1"), pane("p2"))
	got, err := ComputeGoldenRatio(root, "p2", 0.999)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ratio < minRatio {
		t.Errorf("ratio %v below clamp %v", got.Ratio, minRatio)
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
