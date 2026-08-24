# Development

```sh
make test      # unit tests for the ratio/tree logic
make vet
make e2e       # end-to-end tests against a throwaway Herdr server
```

`make e2e` starts an isolated Herdr server under `/tmp/grx` with its own socket,
plugin registry and config, runs the suite, then tears it down. It never touches
your real session, layout or plugin config. (The root is under `/tmp` because a
unix socket path has to fit in `sun_path`, about 104 bytes on macOS.)

The suite refuses to run if `XDG_CONFIG_HOME` points inside `$HOME`, as a guard
against pointing it at a real session by accident.

## Code layout

| File | Role |
|---|---|
| `layout.go` | `ComputeGoldenRatio` — pure tree math, no I/O |
| `client.go` | Herdr socket client (one request per connection) |
| `config.go` | reads the optional `config.toml` |
| `main.go` | `apply` and `hook` entrypoints, both calling `Apply` |

`ComputeGoldenRatio` takes a layout tree and a pane id and returns the split path
and ratio to set — no sockets, no environment. Wiring up a new trigger means
calling `Apply` and nothing else.

## Notes on the Herdr API

Things worth knowing if you extend this, all verified against Herdr 0.8.0 rather
than taken from the docs:

- `pane.focused` **is** a valid manifest event hook. An unrecognised event name
  produces a `warnings` entry on `herdr plugin link`, which is a handy way to
  check any event name.
- The `pane_focused` payload carries `pane_id` and `workspace_id` but **no
  `tab_id`**, so the tab has to be resolved from the pane. `layout.export`
  accepts a `pane_id` and returns the tab id, focused pane and full tree in one
  call.
- `layout.set_split_ratio` takes `path` (a bool array: `[]` is the root split,
  `false` descends into `first`, `true` into `second`) and `ratio`, which is
  always **the first child's share** — a focused pane on the `second` side needs
  `1 - ratio`.
- `pane.resize` is a *relative* nudge, so it cannot express "make this exactly
  61.8%". `layout.set_split_ratio` is the right primitive.
- The socket serves **one request per connection** and closes; reconnect per
  call.
- `herdr plugin config-dir` resolves the path client-side from
  `XDG_CONFIG_HOME`, not from the server being addressed. This matters when
  testing against a second server.
- Focusing a pane that already has focus emits no `pane_focused` event.
