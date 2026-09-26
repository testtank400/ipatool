package main

import (
	"context"
	"errors"
	"os"

	"github.com/majd/ipatool/v2/internal/gui"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails-bound facade over gui.Service.
type App struct {
	ctx context.Context
	svc *gui.Service
}

func NewApp(svc *gui.Service) *App {
	return &App{svc: svc}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Login authenticates with the App Store (may take minutes on first SAP setup).
func (a *App) Login(email, password, authCode, keychainPassphrase string) (gui.LoginResult, error) {
	return a.svc.Login(email, password, authCode, keychainPassphrase)
}

// AccountInfo returns the signed-in account summary.
func (a *App) AccountInfo() (gui.AccountSummary, error) {
	return a.svc.AccountInfo()
}

// Revoke clears stored credentials.
func (a *App) Revoke() error {
	return a.svc.Revoke()
}

// Search finds App Store apps. Bound for the frontend.
func (a *App) Search(term string, limit int, platform string) (gui.SearchResult, error) {
	return a.svc.Search(term, int64(limit), platform)
}

// ListVersions lists available versions for an app. resolve fetches display version + date (slow).
func (a *App) ListVersions(appID int64, bundleID string, resolve bool, page, maxResults int, platform string) (gui.ListVersionsResult, error) {
	return a.svc.ListVersions(appID, bundleID, resolve, page, maxResults, platform)
}

// SelectOutputPath opens a native directory dialog; download writes {bundle}_{id}_{ver}.ipa into it.
// Returns empty string if the user cancels.
func (a *App) SelectOutputPath() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select download folder",
	})
}

// OutputDirectoryExists reports whether path is an existing directory. Missing
// paths are a normal false result so stale frontend settings can be cleared.
func (a *App) OutputDirectoryExists(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// Download downloads an IPA then replicates sinfs.
// Progress strategy (v1): Progress bar is nil in pkg/appstore; this method emits
// coarse "download:start" / "download:done" / "download:error" events via EventsEmit.
func (a *App) Download(appID int64, bundleID, output, externalVersionID, platform string) (gui.DownloadResult, error) {
	runtime.EventsEmit(a.ctx, "download:start", map[string]interface{}{
		"appID":    appID,
		"bundleID": bundleID,
		"output":   output,
	})
	result, err := a.svc.Download(appID, bundleID, output, externalVersionID, platform)
	if err != nil {
		runtime.EventsEmit(a.ctx, "download:error", map[string]interface{}{
			"error": err.Error(),
		})
		return gui.DownloadResult{}, err
	}
	runtime.EventsEmit(a.ctx, "download:done", map[string]interface{}{
		"destinationPath": result.DestinationPath,
	})
	return result, nil
}

// SetKeychainPassphrase unlocks the file keyring for subsequent AccountInfo/Search/Download.
func (a *App) SetKeychainPassphrase(passphrase string) {
	a.svc.SetKeychainPassphrase(passphrase)
}
