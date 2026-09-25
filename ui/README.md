# Desktop UI (Wails v2)

Vanilla HTML/CSS/JS frontend. Go bindings live in this module; App Store logic is in
`../internal/gui` (parent module `github.com/majd/ipatool/v2` via `replace`).

## Module layout

- Parent: `github.com/majd/ipatool/v2` (CLI + `internal/gui` + `pkg/...`)
- UI: `github.com/majd/ipatool/v2/ui` with `replace github.com/majd/ipatool/v2 => ../`
- Least pain with Wails (wants its own `go.mod` next to `wails.json`)

## Progress strategy (v1)

`DownloadInput.Progress` is passed as `nil`. `pkg/appstore.downloadFile` already
supports nil (plain `io.Copy`, no progressbar). The Wails layer emits coarse events:
`download:start`, `download:done`, `download:error` via `runtime.EventsEmit`.
No CLI / progressbar API changes.

## Build

```
cd ui
go mod tidy
go build .
wails build -skipbindings
```

Full `wails build` (without `-skipbindings`) compiles and runs `wailsbindings.exe`
from TEMP; on this machine Application Control blocked that helper. Runtime Go
bindings still work via `window.go.main.App` without generated TS modules.

Dev: `wails dev -skipbindings` from `ui/` (if App Control allows the temp helper,
omit `-skipbindings`).

Output: `build/bin/ipatool-ui.exe`
