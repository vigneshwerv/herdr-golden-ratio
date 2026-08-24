package main

import (
	"errors"
	"fmt"
)

// Golden is the reciprocal of the golden ratio: 1/phi == phi-1 == 0.6180339...
// The focused pane's side of its parent split gets this fraction of the space.
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
// of its immediate parent split.
//
// Only the immediate parent split is touched. Propagating the ratio up the tree
// would compound it (a pane nested three levels deep would end up with
// 0.618^3, about 24% of the tab — shrinking the more nested it is, the opposite
// of the intent) and would disturb splits containing panes the user never
// focused. Adjusting one split is also the least surprising thing to undo.
//
// The returned Resize is a pure function of its inputs; it performs no I/O.
func ComputeGoldenRatio(root *Node, focusedPaneID string, ratio float64) (Resize, error) {
	if root == nil {
		return Resize{}, ErrPaneNotFound
	}
	if ratio <= 0 || ratio >= 1 {
		return Resize{}, fmt.Errorf("ratio %v out of range (0,1)", ratio)
	}

	path, ok := findPane(root, focusedPaneID)
	if !ok {
		return Resize{}, ErrPaneNotFound
	}
	if len(path) == 0 {
		// The pane is the tree root, so the tab holds a single pane.
		return Resize{}, ErrSinglePane
	}

	// The last step of the path into the pane tells us which side of its
	// parent split the pane sits on; everything before it addresses the
	// split itself.
	parentPath := path[:len(path)-1]
	focusedIsSecond := path[len(path)-1]

	// set_split_ratio always specifies the First child's share, so a focused
	// pane on the Second side needs the complement.
	target := ratio
	if focusedIsSecond {
		target = 1 - ratio
	}
	target = clamp(target, minRatio, maxRatio)

	parent := nodeAt(root, parentPath)
	if parent == nil || parent.Type != "split" {
		return Resize{}, ErrPaneNotFound
	}

	return Resize{
		Path:            parentPath,
		Ratio:           target,
		Current:         parent.Ratio,
		Direction:       parent.Direction,
		FocusedIsSecond: focusedIsSecond,
	}, nil
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
