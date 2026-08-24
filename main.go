// Command herdr-golden-ratio resizes the focused Herdr pane to occupy the
// golden ratio (~61.8%) of its parent split, shrinking its sibling to match.
//
// It has two entrypoints, both of which reach the same Apply function:
//
//	apply   run once against the currently focused pane (keybinding or menu)
//	hook    handle a pane.focused event from the manifest event hook
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

func main() {
	mode := "apply"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	var err error
	switch mode {
	case "apply":
		err = runApply()
	case "hook":
		err = runHook()
	case "version":
		fmt.Println("herdr-golden-ratio 0.2.0")
	default:
		err = fmt.Errorf("unknown mode %q (want: apply, hook, version)", mode)
	}

	if err != nil {
		// Plugin stderr is captured by `herdr plugin log list`.
		fmt.Fprintf(os.Stderr, "herdr-golden-ratio: %v\n", err)
		os.Exit(1)
	}
}

// actionContext is the subset of HERDR_PLUGIN_CONTEXT_JSON we need. Herdr
// resolves the focused pane for us, which is more reliable than HERDR_PANE_ID:
// the latter is the pane the command inherited, which for a CLI invocation is
// the caller's own pane rather than the pane the user is looking at.
type actionContext struct {
	FocusedPaneID string `json:"focused_pane_id"`
	TabID         string `json:"tab_id"`
}

// runApply handles an explicit invocation: a keybinding or the action menu.
func runApply() error {
	cfg := LoadConfig()
	client, err := NewClient()
	if err != nil {
		return err
	}

	// Prefer the focused pane Herdr reports in the action context, falling back
	// to the inherited pane id when the context is absent.
	target := os.Getenv("HERDR_PANE_ID")
	if raw := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); raw != "" {
		var ctx actionContext
		if err := json.Unmarshal([]byte(raw), &ctx); err == nil && ctx.FocusedPaneID != "" {
			target = ctx.FocusedPaneID
		}
	}

	layout, err := client.ExportLayout(target)
	if err != nil {
		if IsNotFound(err) {
			// The pane or tab went away between the keypress and this read.
			return nil
		}
		return err
	}

	// Trust the server's own view of focus within that tab, so the resize
	// follows the user's cursor even if the action was bound oddly.
	focused := layout.FocusedPaneID
	if focused == "" {
		focused = target
	}
	return Apply(client, cfg, layout, focused)
}

// paneFocusedEvent is the payload delivered in HERDR_PLUGIN_EVENT_JSON for a
// pane.focused hook. Note that it carries no tab_id, so the tab must be
// resolved from the pane.
type paneFocusedEvent struct {
	Event string `json:"event"`
	Data  struct {
		Type        string `json:"type"`
		PaneID      string `json:"pane_id"`
		WorkspaceID string `json:"workspace_id"`
	} `json:"data"`
}

// runHook handles a pane.focused event fired by the manifest event hook.
func runHook() error {
	cfg := LoadConfig()
	if !cfg.Auto {
		// The hook is registered but disabled by default. Exit quietly so we
		// do not spam the plugin log on every focus change.
		return nil
	}

	raw := os.Getenv("HERDR_PLUGIN_EVENT_JSON")
	if raw == "" {
		return errors.New("HERDR_PLUGIN_EVENT_JSON is empty; not invoked as an event hook")
	}
	var ev paneFocusedEvent
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		return fmt.Errorf("decode event: %w", err)
	}
	paneID := ev.Data.PaneID
	if paneID == "" {
		return errors.New("event carried no pane_id")
	}

	client, err := NewClient()
	if err != nil {
		return err
	}

	// Coalesce bursts of focus changes. Herdr spawns one process per event, so
	// rather than sharing debounce state between processes we wait, then drop
	// out if focus has already moved on. Whichever process is holding the
	// final focus does the work; the rest exit silently.
	if cfg.Debounce > 0 {
		time.Sleep(cfg.Debounce)
	}

	layout, err := client.ExportLayout(paneID)
	if err != nil {
		if IsNotFound(err) {
			// The pane closed while we were debouncing.
			return nil
		}
		return err
	}
	if layout.FocusedPaneID != paneID {
		return nil // Focus moved on; a later invocation owns this.
	}

	return Apply(client, cfg, layout, paneID)
}

// Apply gives focusedPaneID the configured fraction of its immediate parent
// split. It is the single place both entrypoints converge on, so wiring up a
// new trigger means calling this and nothing else.
func Apply(client *Client, cfg Config, layout *LayoutDescription, focusedPaneID string) error {
	if layout.Zoomed {
		// A zoomed pane already fills the tab and the ratio is invisible;
		// resizing underneath it would surprise the user on unzoom.
		return nil
	}
	if focusedPaneID == "" {
		return errors.New("could not determine the focused pane")
	}

	steps, err := ComputeGoldenRatio(layout.Root, focusedPaneID, cfg.Ratio)
	if err != nil {
		if errors.Is(err, ErrSinglePane) || errors.Is(err, ErrPaneNotFound) {
			// Nothing to do: a lone pane in a tab, or a pane that has gone
			// away. Both are ordinary, not failures.
			return nil
		}
		return err
	}

	// Apply from the root down. Each split's ratio is independent of the
	// others, so order only affects what the user sees mid-flight; going
	// outermost-first avoids a visible bounce.
	for _, s := range steps {
		// Skip a write that would not visibly change anything.
		if abs(s.Current-s.Ratio) < cfg.MinDelta {
			continue
		}
		if err := client.SetSplitRatio(layout.TabID, s.Path, s.Ratio); err != nil {
			if IsNotFound(err) {
				return nil // The tab went away mid-resize.
			}
			return err
		}
	}
	return nil
}
