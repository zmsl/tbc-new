# WoWSims TBC desktop shell

An Electron wrapper around the existing `wowsimtbc` binary. It owns a window, starts the sim
server as a child process, and shuts that server down when the window closes.

This is a **separate npm project on purpose**. Electron and electron-builder are ~200MB of
dependencies that nothing else in the repo needs, and adding them to the root
`package.json` would slow `npm ci` for every contributor and every CI job. Nothing here is
on the default build path — `make`, `make test` and `make host` never touch it.

## How it fits together

```
Electron main  ──spawn──▶  wowsimtbc --desktop
      │                          │
      │  ◀── stdout: "WOWSIMS_LISTENING port=54312"
      │
      ├─ protocol.handle('wowsims') ──▶ http://127.0.0.1:54312
      └─ BrowserWindow.loadURL('wowsims://app/tbc/')
```

The Go binary rides inside the app bundle as an `extraResources` sidecar, so **one update
updates both** the shell and the sim engine.

### Why a custom `wowsims://` scheme instead of just loading the localhost URL

Every saved setting and gear set lives in `window.localStorage`, which browsers key by
origin **including the port**. The sim server binds an OS-assigned port so that a port
conflict can never stop the app from starting — which means loading
`http://127.0.0.1:<port>` directly would hand the user an empty settings store on every
single launch. Proxying a fixed scheme to whatever port we got keeps the origin stable.

Two consequences worth knowing:

- `ui/core/utils.ts` treats the `wowsims:` protocol as a local environment. Without that the
  UI decides it is the public website and offers to sell you the downloadable sim you are
  already running.
- Only one instance may run. Two windows would share the `wowsims://app` origin and
  therefore the same localStorage, so a second launch focuses the existing window instead.

## Development

```sh
make desktop-dev          # builds wowsimtbc at the repo root, then runs the shell against it
```

Or by hand, once `make wowsimtbc` has produced the root binary:

```sh
cd desktop && npm install && npm start
```

In an unpackaged run the shell looks for `../wowsimtbc`; when packaged it uses
`process.resourcesPath`.

## Building installers

```sh
VERSION=v1.2.3 make desktop-win     # NSIS installer   -> desktop-dist/
VERSION=v1.2.3 make desktop-mac     # dmg + zip        -> desktop-dist/
```

`VERSION` matters: electron-updater compares releases against the version in
`package.json`, while the sim server reports its `-ldflags` value. If the two drift, the app
either offers an update it already has or never notices one. The make targets keep them in
sync; CI does the same via `npm version`.

To look at the real window without building an installer (no wine needed -- this stops
before NSIS assembly):

```sh
make desktop-preview-win        # -> desktop-dist/win-unpacked/
```

Copy that directory somewhere under `/mnt/c` and run `WoWSims TBC.exe` from Windows.

Installers are built on their own platform in CI (`.github/workflows/release.yml`) — dmg
creation requires macOS, and building NSIS on Windows leaves room for code signing later.

## Auto-update

Prompted, not silent: `autoDownload` is off, and the toolbar control in the sim UI drives
`available -> downloading -> ready`, installing on the final click.

**macOS auto-update requires code signing.** electron-updater's macOS path is Squirrel.Mac,
which refuses to apply an update to an unsigned bundle; there is no workaround. Until a
Developer ID certificate is wired in, mac builds report `canSelfUpdate: false` and the
control opens the releases page instead. Flip `MAC_AUTO_UPDATE_SUPPORTED` in `main.js` once
signing exists. Windows NSIS auto-update works unsigned (SmartScreen will still warn on the
installer until a certificate builds reputation).

`publish.owner` in `electron-builder.yml` must point at the fork that publishes the
installers. Pointing it at upstream would make the first update silently replace this build
with an upstream one.

## Migrating settings from the browser

The desktop app has its own origin, so it cannot see settings saved at `wowsims.com` or
`localhost:3333`. Use the sim's existing JSON export in the browser and the matching import
in the app.
