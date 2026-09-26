package gui

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/byteness/keyring"
	cookiejar "github.com/juju/persistent-cookiejar"
	"github.com/majd/ipatool/v2/pkg/appstore"
	"github.com/majd/ipatool/v2/pkg/http"
	"github.com/majd/ipatool/v2/pkg/keychain"
	"github.com/majd/ipatool/v2/pkg/util/machine"
	"github.com/majd/ipatool/v2/pkg/util/operatingsystem"
)

const (
	configDirectoryName = ".ipatool"
	cookieJarFileName   = "cookies"
	keychainServiceName = "ipatool-auth.service"
)

// Service mirrors CLI DI from cmd/common.go without importing cmd.
type Service struct {
	mu         sync.Mutex
	passphrase string

	os        operatingsystem.OperatingSystem
	machine   machine.Machine
	cookieJar http.CookieJar
	keychain  keychain.Keychain
	appStore  appstore.AppStore
}

// New constructs a Service with OS/machine/cookie jar/keychain/AppStore
// wired like the CLI (cookies at ~/.ipatool/cookies, keychain service ipatool-auth.service).
func New() (*Service, error) {
	s := &Service{}
	s.os = operatingsystem.New()
	s.machine = machine.New(machine.Args{OS: s.os})
	if err := createConfigDirectory(s.os, s.machine); err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: filepath.Join(s.machine.HomeDirectory(), configDirectoryName, cookieJarFileName),
	})
	if err != nil {
		return nil, fmt.Errorf("cookie jar: %w", err)
	}
	s.cookieJar = jar
	if err := s.openKeychain(); err != nil {
		return nil, err
	}
	s.appStore = appstore.NewAppStore(appstore.Args{
		CookieJar:       s.cookieJar,
		OperatingSystem: s.os,
		Keychain:        s.keychain,
		Machine:         s.machine,
	})
	return s, nil
}

func (s *Service) openKeychain() error {
	ring, err := keyring.Open(keyring.Config{
		AllowedBackends: []keyring.BackendType{
			keyring.KeychainBackend,
			keyring.SecretServiceBackend,
			keyring.FileBackend,
		},
		ServiceName:              keychainServiceName,
		KeychainTrustApplication: true,
		FileDir:                  filepath.Join(s.machine.HomeDirectory(), configDirectoryName),
		FilePasswordFunc: func(string) (string, error) {
			s.mu.Lock()
			pass := s.passphrase
			s.mu.Unlock()
			if pass == "" {
				return "", errors.New("keychain passphrase is required")
			}
			return pass, nil
		},
	})
	if err != nil {
		return fmt.Errorf("keychain: %w", err)
	}
	s.keychain = keychain.New(keychain.Args{
		Keyring: ring,
		Label:   keychainServiceName,
	})
	return nil
}

func createConfigDirectory(os operatingsystem.OperatingSystem, machine machine.Machine) error {
	configDirectoryPath := filepath.Join(machine.HomeDirectory(), configDirectoryName)
	_, err := os.Stat(configDirectoryPath)
	if err != nil && os.IsNotExist(err) {
		if err = os.MkdirAll(configDirectoryPath, 0700); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("could not read metadata: %w", err)
	}
	return nil
}

// SetKeychainPassphrase stores the file-backend unlock passphrase used by keyring.
func (s *Service) SetKeychainPassphrase(passphrase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.passphrase = passphrase
}

// AccountSummary is a frontend-safe view of the signed-in account.
type AccountSummary struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// AppSummary is a frontend-safe search/download target.
type AppSummary struct {
	ID       int64   `json:"id"`
	BundleID string  `json:"bundleID"`
	Name     string  `json:"name"`
	Version  string  `json:"version"`
	Price    float64 `json:"price"`
}

// LoginResult is returned to the frontend after a successful login.
type LoginResult struct {
	Account AccountSummary `json:"account"`
}

// SearchResult is returned to the frontend after a search.
type SearchResult struct {
	Count int          `json:"count"`
	Apps  []AppSummary `json:"apps"`
}

// DownloadResult is returned after a successful download + sinf replicate.
type DownloadResult struct {
	DestinationPath string `json:"destinationPath"`
}

// VersionSummary is a frontend-safe version entry.
type VersionSummary struct {
	ExternalVersionID string `json:"externalVersionID"`
	DisplayVersion    string `json:"displayVersion,omitempty"`
	ReleaseDate       string `json:"releaseDate,omitempty"`
	Error             string `json:"error,omitempty"`
}

// ListVersionsResult is returned after listing available app versions.
type ListVersionsResult struct {
	ExternalVersionIdentifiers []string         `json:"externalVersionIdentifiers"`
	LatestExternalVersionID    string           `json:"latestExternalVersionID"`
	Versions                   []VersionSummary `json:"versions"`
	Page                       int              `json:"page"`
	TotalCount                 int              `json:"totalCount"`
	// ResolveError is set when version IDs were loaded but detail resolve failed.
	// The frontend should still show IDs and surface this message.
	ResolveError string `json:"resolveError,omitempty"`
}

// Login authenticates with the App Store. keychainPassphrase unlocks ~/.ipatool keyring.
func (s *Service) Login(email, password, authCode, keychainPassphrase string) (LoginResult, error) {
	if keychainPassphrase != "" {
		s.SetKeychainPassphrase(keychainPassphrase)
	}
	out, err := s.appStore.Login(appstore.LoginInput{
		Email:    email,
		Password: password,
		AuthCode: authCode,
	})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Account: AccountSummary{Email: out.Account.Email, Name: out.Account.Name}}, nil
}

// AccountInfo returns the currently stored account.
func (s *Service) AccountInfo() (AccountSummary, error) {
	out, err := s.appStore.AccountInfo()
	if err != nil {
		return AccountSummary{}, err
	}
	return AccountSummary{Email: out.Account.Email, Name: out.Account.Name}, nil
}

// Revoke clears stored credentials.
func (s *Service) Revoke() error {
	return s.appStore.Revoke()
}

// Search finds apps matching term. platform is iphone|ipad|appletv|visionos (or empty).
func (s *Service) Search(term string, limit int64, platform string) (SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	info, err := s.appStore.AccountInfo()
	if err != nil {
		return SearchResult{}, err
	}
	plat, err := appstore.ParsePlatform(platform)
	if err != nil {
		return SearchResult{}, err
	}
	out, err := s.appStore.Search(appstore.SearchInput{
		Account:  info.Account,
		Term:     term,
		Limit:    limit,
		Platform: plat,
	})
	if err != nil {
		return SearchResult{}, err
	}
	apps := make([]AppSummary, 0, len(out.Results))
	for _, a := range out.Results {
		apps = append(apps, AppSummary{
			ID: a.ID, BundleID: a.BundleID, Name: a.Name, Version: a.Version, Price: a.Price,
		})
	}
	return SearchResult{Count: out.Count, Apps: apps}, nil
}

// ListVersions lists available external version IDs for an app.
// When resolve is true, display version + release date are fetched per page (slow).
// With resolve, maxResults of 0 is coerced to DefaultVersionResolvePageSize so the
// UI never tries to resolve an entire version history in one call.
func (s *Service) ListVersions(appID int64, bundleID string, resolve bool, page, maxResults int, platform string) (ListVersionsResult, error) {
	if appID == 0 && bundleID == "" {
		return ListVersionsResult{}, errors.New("either the app ID or the bundle identifier must be specified")
	}
	if page < 1 {
		page = 1
	}
	if maxResults < 0 {
		return ListVersionsResult{}, errors.New("max-results must not be negative")
	}
	if resolve && maxResults == 0 {
		maxResults = appstore.DefaultVersionResolvePageSize
	}

	info, err := s.appStore.AccountInfo()
	if err != nil {
		return ListVersionsResult{}, err
	}
	acc := info.Account
	plat, err := appstore.ParsePlatform(platform)
	if err != nil {
		return ListVersionsResult{}, err
	}

	// Prefer an explicit app ID so a flaky iTunes lookup cannot block listing.
	// Bundle ID is still resolved when it is the only identifier provided.
	app := appstore.App{ID: appID}
	if bundleID != "" && appID == 0 {
		lookup, err := s.appStore.Lookup(appstore.LookupInput{
			Account:  acc,
			BundleID: bundleID,
			Platform: plat,
		})
		if err != nil {
			return ListVersionsResult{}, err
		}
		app = lookup.App
	} else if bundleID != "" {
		app.BundleID = bundleID
	}

	// IDs-first: always load the identifier page without metadata so a resolve
	// failure cannot wipe the version list.
	idOut, err := s.appStore.ListVersions(appstore.ListVersionsInput{
		Account:         acc,
		App:             app,
		Platform:        plat,
		ResolveMetadata: false,
		Page:            page,
		MaxResults:      maxResults,
	})
	if err != nil {
		return ListVersionsResult{}, err
	}

	result := ListVersionsResult{
		ExternalVersionIdentifiers: idOut.ExternalVersionIdentifiers,
		LatestExternalVersionID:    idOut.LatestExternalVersionID,
		Page:                       idOut.Page,
		TotalCount:                 idOut.TotalCount,
		Versions:                   []VersionSummary{},
	}

	if !resolve {
		return result, nil
	}

	resolved, resolveErr := s.appStore.ListVersions(appstore.ListVersionsInput{
		Account:         acc,
		App:             app,
		Platform:        plat,
		ResolveMetadata: true,
		Page:            page,
		MaxResults:      maxResults,
	})
	if resolveErr != nil {
		// Keep IDs from the first call; attach whatever partial versions we got.
		result.ResolveError = resolveErr.Error()
		if len(resolved.ExternalVersionIdentifiers) > 0 {
			result.ExternalVersionIdentifiers = resolved.ExternalVersionIdentifiers
			result.LatestExternalVersionID = resolved.LatestExternalVersionID
			result.Page = resolved.Page
			result.TotalCount = resolved.TotalCount
		}
		result.Versions = mapVersionSummaries(resolved.Versions)
		return result, nil
	}

	result.ExternalVersionIdentifiers = resolved.ExternalVersionIdentifiers
	result.LatestExternalVersionID = resolved.LatestExternalVersionID
	result.Page = resolved.Page
	result.TotalCount = resolved.TotalCount
	result.Versions = mapVersionSummaries(resolved.Versions)
	return result, nil
}

func mapVersionSummaries(versions []appstore.ListedVersion) []VersionSummary {
	out := make([]VersionSummary, 0, len(versions))
	for _, v := range versions {
		vs := VersionSummary{
			ExternalVersionID: v.ExternalVersionID,
			DisplayVersion:    v.DisplayVersion,
			Error:             v.Error,
		}
		if !v.ReleaseDate.IsZero() {
			vs.ReleaseDate = v.ReleaseDate.UTC().Format(time.RFC3339)
		}
		out = append(out, vs)
	}
	return out
}

// Download downloads an IPA (Progress is nil - coarse start/done events belong in the UI layer)
// then replicates sinfs like cmd/download.go.
func (s *Service) Download(appID int64, bundleID, output, externalVersionID, platform string) (DownloadResult, error) {
	if appID == 0 && bundleID == "" {
		return DownloadResult{}, errors.New("either the app ID or the bundle identifier must be specified")
	}
	info, err := s.appStore.AccountInfo()
	if err != nil {
		return DownloadResult{}, err
	}
	acc := info.Account
	plat, err := appstore.ParsePlatform(platform)
	if err != nil {
		return DownloadResult{}, err
	}
	app := appstore.App{ID: appID}
	if bundleID != "" {
		lookup, err := s.appStore.Lookup(appstore.LookupInput{
			Account:  acc,
			BundleID: bundleID,
			Platform: plat,
		})
		if err != nil {
			return DownloadResult{}, err
		}
		app = lookup.App
	}
	out, err := s.appStore.Download(appstore.DownloadInput{
		Account:           acc,
		App:               app,
		OutputPath:        output,
		Progress:          nil, // coarse start/done events emitted by Wails UI layer
		ExternalVersionID: externalVersionID,
		Platform:          plat,
	})
	if err != nil {
		return DownloadResult{}, err
	}
	if err := s.appStore.ReplicateSinf(appstore.ReplicateSinfInput{
		Sinfs:       out.Sinfs,
		PackagePath: out.DestinationPath,
	}); err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{DestinationPath: out.DestinationPath}, nil
}
