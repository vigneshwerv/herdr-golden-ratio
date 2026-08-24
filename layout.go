package main

import (
	"errors"
	"fmt"
	"math"
)

// Golden is the reciprocal of the golden ratio: 1/phi == phi-1 == 0.6180339...
// The focused pane gets this fraction of the tab along each axis it is split on.
const Golden = 0.6180339887498949

// Node is one node in a tab's binary (BSP) layout tree, as returned by the
// layout.export socket method. A node is either a leaf ("pane") or an internal
// split ("split") with exactly two children.
type Node struct {
	Type string `json:"type"`

	// Leaf ("pane") fields.
	PaneID string `json:"pane_id,omitempty"`

	// Internal ("split") fields. Direction is "right" (side by side) or
	// "down" (stacked). Ratio is the fraction of the split's space given to
	// First; Second implicitly receives 1-Ratio.
	Direction string  `json:"direction,omitempty"`
	Ratio     float64 `json:"ratio,omitempty"`
	First     *Node   `json:"first,omitempty"`
	Second    *Node   `json:"second,omitempty"`
}

// LayoutDescription is the result payload of layout.export.
type LayoutDescription struct {
	WorkspaceID   string `json:"workspace_id"`
	TabID         string `json:"tab_id"`
	Zoomed        bool   `json:"zoomed"`
	FocusedPaneID string `json:"focused_pane_id"`
	Root          *Node  `json:"root"`
}

// ErrSinglePane means the pane is the tab's root: there is no parent split to
// resize, so the golden ratio is undefined (and unnecessary — it already fills
// the tab). Callers treat this as a no-op, not a failure.
var ErrSinglePane = errors.New("pane is the only pane in its tab; no split to resize")

// ErrPaneNotFound means the pane is not present in the supplied tree, which
// normally means it closed or moved between the event firing and our read.
var ErrPaneNotFound = errors.New("pane not found in layout tree")

// Resize is an absolute instruction for the layout.set_split_ratio method:
// set the split reached by Path to Ratio.
type Resize struct {
	// Path locates the split within the tree. Empty means the root split;
	// false descends into First, true into Second.
	Path []bool
	// Ratio is the fraction to give the split's First child.
	Ratio float64
	// Current is the split's existing ratio, for "already golden" checks.
	Current float64
	// Direction is the parent split's orientation, for logging.
	Direction string
	// FocusedIsSecond reports which side of the split the focused pane is on.
	FocusedIsSecond bool
}

// minRatio/maxRatio keep a split from collapsing a pane to nothing. Herdr
// enforces its own minimum cell sizes, but clamping here keeps our intent
// explicit and the resulting ratio meaningful.
const (
	minRatio = 0.05
	maxRatio = 0.95
)

// ComputeGoldenRatio works out how to give focusedPaneID the fraction `ratio`
// of the whole tab, along each axis it is split on.
//
// Adjusting only the pane's immediate parent split is not enough. Splitting
// right twice does not produce three siblings; it nests:
//
//	root(right){ A, inner(right){ B, C } }
//
// Setting only `inner` gives B 0.618 of `inner`, and `inner` is just half the
// tab, so B ends up at 0.618*0.5 = 31%. Every ancestor left at its old ratio
// scales the result down.
//
// So every split between the root and the pane is adjusted. Splits are grouped
// by axis, because "right" splits divide width and "down" splits divide height
// and the two are independent. Within an axis crossed k times, each split gives
// the focused side ratio**(1/k), so the product across them is exactly `ratio`:
// the pane ends up with 61.8% of the tab's width and 61.8% of its height,
// regardless of how deeply it is nested.
//
// The returned steps are a pure function of the inputs; this performs no I/O.
func ComputeGoldenRatio(root *Node, focusedPaneID string, ratio float64) ([]Resize, error) {
	if root == nil {
		return nil, ErrPaneNotFound
	}
	if ratio <= 0 || ratio >= 1 {
		return nil, fmt.Errorf("ratio %v out of range (0,1)", ratio)
	}

	path, ok := findPane(root, focusedPaneID)
	if !ok {
		return nil, ErrPaneNotFound
	}
	if len(path) == 0 {
		// The pane is the tree root, so the tab holds a single pane.
		return nil, ErrSinglePane
	}

	// Walk root -> pane, recording each split crossed and which side the
	// focused pane went down.
	type step struct {
		path      []bool
		split     *Node
		wentRight bool // took the Second child
	}
	steps := make([]step, 0, len(path))
	cur := root
	for i, second := range path {
		if cur == nil || cur.Type != "split" {
			return nil, ErrPaneNotFound
		}
		steps = append(steps, step{path: path[:i:i], split: cur, wentRight: second})
		if second {
			cur = cur.Second
		} else {
			cur = cur.First
		}
	}

	// How many times each axis is crossed decides each split's share.
	crossings := map[string]int{}
	for _, s := range steps {
		crossings[s.split.Direction]++
	}

	out := make([]Resize, 0, len(steps))
	for _, s := range steps {
		k := crossings[s.split.Direction]
		// The k-th root, so k splits on this axis multiply out to `ratio`.
		share := math.Pow(ratio, 1/float64(k))

		// set_split_ratio always specifies the First child's share, so a
		// focused pane on the Second side needs the complement.
		target := share
		if s.wentRight {
			target = 1 - share
		}

		out = append(out, Resize{
			Path:            s.path,
			Ratio:           clamp(target, minRatio, maxRatio),
			Current:         s.split.Ratio,
			Direction:       s.split.Direction,
			FocusedIsSecond: s.wentRight,
		})
	}
	return out, nil
}

// findPane returns the sequence of child selections leading from root to the
// pane with the given id: false for First, true for Second. A nil slice with
// ok==true means the pane is the root itself.
func findPane(n *Node, paneID string) (path []bool, ok bool) {
	if n == nil {
		return nil, false
	}
	if n.Type == "pane" {
		if n.PaneID == paneID {
			return nil, true
		}
		return nil, false
	}
	if sub, found := findPane(n.First, paneID); found {
		return append([]bool{false}, sub...), true
	}
	if sub, found := findPane(n.Second, paneID); found {
		return append([]bool{true}, sub...), true
	}
	return nil, false
}

// nodeAt walks a path of child selections from root and returns the node found,
// or nil if the path leaves the tree.
func nodeAt(root *Node, path []bool) *Node {
	cur := root
	for _, second := range path {
		if cur == nil || cur.Type != "split" {
			return nil
		}
		if second {
			cur = cur.Second
		} else {
			cur = cur.First
		}
	}
	return cur
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
