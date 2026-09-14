# herdr-golden-ratio

Automatically resize the focused [Herdr](https://herdr.dev) pane to occupy
the golden ratio (~61.8%) of the tab. It's a Herdr-port of the
vim `golden-ratio` plugin.

Works two ways:

- **Manual** — press a key, the focused pane resizes. This is the default.
- **Automatic** — every focus change re-balances the layout.
  Opt in with `auto = true` (see [Additional configuration](#additional-configuration)).

This plugin only modifies split ratios. Panes are never torn down or created.

> [!IMPORTANT]
> **Automatic mode does not work on Herdr 0.9.0 — this is a Herdr bug, not a
> plugin bug.** Manual mode (the keybinding) is unaffected.
>
> 0.9.0 stopped emitting the `pane.focused` event when you navigate panes from
> the UI, so the plugin's event hook is never invoked. Herdr still updates its
> own focus state, it just does not tell plugins about it — which is why nothing
> appears in `herdr plugin log list` even though the plugin is installed and
> enabled correctly.
>
> Tracked upstream as [herdrdev/herdr#3824][bug]. It is **fixed on master and
> marked `pending-release`**, but as of Herdr 0.9.0 no published release —
> stable or preview — contains the fix.
>
> Until a fixed release ships, set `auto = false` and use the keybinding. No
> plugin change is needed when you do upgrade: the event hook is already
> declared, so automatic mode starts working again on its own.

[bug]: https://github.com/herdrdev/herdr/issues/3824

## Requirements

- Herdr **0.8.0** or newer (needs the `layout.set_split_ratio` method and the
  `pane.focused` event hook).
- Go 1.24+ to build.

## Install

```sh
git clone https://github.com/vigneshwerv/herdr-golden-ratio ~/src/herdr-golden-ratio
cd ~/src/herdr-golden-ratio
make build                       # produces bin/herdr-golden-ratio
herdr plugin link "$PWD"
```

`herdr plugin link` registers a local directory as a plugin, which is the normal
way to run one when you are developing. To confirm registrations, run:

```sh
herdr plugin list --plugin vv.golden-ratio --json
```

To install from GitHub:

```sh
herdr plugin install vigneshwerv/herdr-golden-ratio
```

## Bind a key

In your Herdr config:

```toml
[[keys.command]]
key = "prefix+f"
type = "plugin_action"
command = "vv.golden-ratio.apply"
description = "golden ratio"
```

Then reload the running server:

```sh
herdr server reload-config
```

## Additional configuration

```toml
# Fraction of the tab given to the focused pane.
ratio = 0.618

# Re-balance on every focus change, with no keypress. Off by default.
auto = false

# How long the automatic hook waits before acting, coalescing bursts of
# focus changes (milliseconds).
debounce_ms = 120

# Skip a resize when the split is already this close to the target.
min_delta = 0.02
```

Config is read on each invocation, so changes take effect immediately.

### Turning on automatic mode

> [!WARNING]
> On Herdr 0.9.0 this has no effect — see the note at the top of this README and
> [herdrdev/herdr#3824][bug]. The keybinding still works.

Set `auto = true` and you are done; the event hook is already declared in the
manifest. It is off by default because Herdr spawns a fresh process on every
focus change, so it is not enabled by default.

## Behaviour

Which split gets resized, and the cases that intentionally do nothing, are
documented in [DOCS.md](DOCS.md).

## Reloading after changes

| What changed | What to run |
|---|---|
| Go source | `make build` — the next invocation picks up the new binary |
| `herdr-plugin.toml` | `herdr plugin unlink vv.golden-ratio && herdr plugin link "$PWD"` |
| Plugin `config.toml` | nothing; read on every invocation |
| Keybinding in `~/.config/herdr/config.toml` | `herdr server reload-config` |

Failed invocations show up in:

```sh
herdr plugin log list --plugin vv.golden-ratio --limit 10
```

## Development

Build, test, code layout and notes on the Herdr API are documented in
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
