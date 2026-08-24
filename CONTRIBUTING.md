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
