package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// Client speaks Herdr's newline-delimited JSON socket protocol.
//
// The server closes the connection after answering a single request, so each
// call dials fresh rather than holding a long-lived connection.
type Client struct {
	socketPath string
}

func NewClient() (*Client, error) {
	p := os.Getenv("HERDR_SOCKET_PATH")
	if p == "" {
		return nil, fmt.Errorf("HERDR_SOCKET_PATH is not set; not running under Herdr")
	}
	return &Client{socketPath: p}, nil
}

type request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *errorBody      `json:"error"`
}

// APIError is a structured error returned by the Herdr server.
type APIError struct {
	Method  string
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", e.Method, e.Message, e.Code)
}

// IsNotFound reports whether err means the target pane, tab or layout no longer
// exists. This is routine rather than exceptional: a pane can close between the
// focus event firing (or a key being pressed) and our read of the layout.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case "layout_not_found", "pane_not_found", "tab_not_found", "not_found":
		return true
	}
	return false
}

// call issues one request and unmarshals result into out (which may be nil).
func (c *Client) call(method string, params any, out any) error {
	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.socketPath, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	if params == nil {
		params = map[string]any{}
	}
	payload, err := json.Marshal(request{
		ID:     "golden-ratio:" + method,
		Method: method,
		Params: params,
	})
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write %s: %w", method, err)
	}

	// Responses can exceed bufio's default buffer on large layouts, so allow
	// the scanner to grow.
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return fmt.Errorf("read %s: %w", method, err)
		}
		return fmt.Errorf("read %s: connection closed without a response", method)
	}

	var resp response
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return fmt.Errorf("decode %s: %w", method, err)
	}
	if resp.Error != nil {
		return &APIError{Method: method, Code: resp.Error.Code, Message: resp.Error.Message}
	}
	if out != nil {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return nil
}

// ExportLayout returns the layout tree of the tab containing paneID. Passing an
// empty paneID targets the currently focused tab.
//
// This single call yields the tab id, the focused pane id and the full tree,
// which matters for the event hook: the pane_focused payload carries only
// pane_id and workspace_id, not tab_id.
func (c *Client) ExportLayout(paneID string) (*LayoutDescription, error) {
	params := map[string]any{}
	if paneID != "" {
		params["pane_id"] = paneID
	}
	var out struct {
		Type   string             `json:"type"`
		Layout *LayoutDescription `json:"layout"`
	}
	if err := c.call("layout.export", params, &out); err != nil {
		return nil, err
	}
	if out.Layout == nil {
		return nil, fmt.Errorf("layout.export returned no layout")
	}
	return out.Layout, nil
}

// SetSplitRatio sets an absolute ratio on one split of a tab's tree.
//
// This is deliberately used instead of pane.resize: pane.resize applies a
// relative delta to a ratio, so it cannot express "make this exactly 61.8%".
// Both only adjust split ratios, so pane identity, running processes and
// scrollback are preserved.
func (c *Client) SetSplitRatio(tabID string, path []bool, ratio float64) error {
	// A nil slice marshals to null; set_split_ratio requires an array, and an
	// empty array is the meaningful "root split" value.
	if path == nil {
		path = []bool{}
	}
	return c.call("layout.set_split_ratio", map[string]any{
		"tab_id": tabID,
		"path":   path,
		"ratio":  ratio,
	}, nil)
}
