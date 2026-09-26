package appstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	versionMetadataWorkers = 8
	// DefaultVersionResolvePageSize is the recommended --max-results when
	// ResolveMetadata is set. Resolving every historical ID at once is too slow
	// (each ID hits the download API and reads Info.plist from the IPA).
	DefaultVersionResolvePageSize = 10
)

type ListVersionsInput struct {
	Account         Account
	App             App
	Platform        Platform
	ResolveMetadata bool
	Page            int
	MaxResults      int
}

type ListVersionsOutput struct {
	ExternalVersionIdentifiers []string
	LatestExternalVersionID    string
	Versions                   []ListedVersion
	Page                       int
	TotalCount                 int
}

func (t *appstore) ListVersions(input ListVersionsInput) (ListVersionsOutput, error) {
	platform := input.Platform
	if platform == "" {
		platform = PlatformIPhone
	}

	switch platform {
	case PlatformIPhone, PlatformIPad, PlatformAppleTV, PlatformVisionOS, PlatformMacOS:
	default:
		return ListVersionsOutput{}, fmt.Errorf("invalid platform %q", platform)
	}

	macAddr, err := t.machine.MacAddress()
	if err != nil {
		return ListVersionsOutput{}, fmt.Errorf("failed to get mac address: %w", err)
	}

	guid := strings.ReplaceAll(strings.ToUpper(macAddr), ":", "")

	output, err := t.listVersionHistory(input, guid, platform)
	if err != nil {
		return output, err
	}

	if !input.ResolveMetadata {
		return output, nil
	}

	output.Versions, err = t.resolveVersionMetadata(input.Account, input.App, guid, platform, output.ExternalVersionIdentifiers)
	if err != nil {
		// Keep identifiers so callers can still show the page of version IDs
		// when metadata resolve fails (token expiry, license, etc.).
		return output, fmt.Errorf("resolve versions: %w", err)
	}

	return output, nil
}

func (t *appstore) listVersionHistory(input ListVersionsInput, guid string, platform Platform) (ListVersionsOutput, error) {
	var externalVersionID string
	var err error

	switch platform {
	case PlatformMacOS:
		externalVersionID, err = t.lookupLatestMacOSExternalVersionID(input.Account, input.App)
	case PlatformAppleTV, PlatformVisionOS:
		externalVersionID, err = t.lookupLatestExternalVersionID(input.Account, input.App, platform)
	}
	if err != nil {
		return ListVersionsOutput{}, fmt.Errorf("failed to resolve platform version: %w", err)
	}

	res, _, err := t.sendDownloadProduct(input.Account, input.App, guid, externalVersionID, platform)
	if err != nil {
		return ListVersionsOutput{}, err
	}

	if err := interpretDownloadResult(res); err != nil {
		return ListVersionsOutput{}, err
	}

	item := res.Data.Items[0]

	rawIdentifiers, ok := item.Metadata["softwareVersionExternalIdentifiers"].([]interface{})
	if !ok {
		return ListVersionsOutput{}, NewErrorWithMetadata(fmt.Errorf("failed to get version identifiers from item metadata"), item.Metadata)
	}

	allIdentifiers := make([]string, len(rawIdentifiers))
	for i, val := range rawIdentifiers {
		allIdentifiers[i] = fmt.Sprintf("%v", val)
	}

	latestExternalVersionID := item.Metadata["softwareVersionExternalIdentifier"]
	if latestExternalVersionID == nil {
		return ListVersionsOutput{}, NewErrorWithMetadata(fmt.Errorf("failed to get latest version from item metadata"), item.Metadata)
	}

	page := input.Page
	if page == 0 {
		page = 1
	}

	pageIdentifiers, err := paginateVersionIDs(allIdentifiers, page, input.MaxResults)
	if err != nil {
		return ListVersionsOutput{}, err
	}

	return ListVersionsOutput{
		ExternalVersionIdentifiers: pageIdentifiers,
		LatestExternalVersionID:    fmt.Sprintf("%v", latestExternalVersionID),
		Page:                       page,
		TotalCount:                 len(allIdentifiers),
	}, nil
}

func paginateVersionIDs(ids []string, page, maxResults int) ([]string, error) {
	if page < 1 {
		return nil, errors.New("page must be greater than 0")
	}

	if maxResults < 0 {
		return nil, errors.New("max-results must not be negative")
	}

	if maxResults == 0 {
		return ids, nil
	}

	newestFirst := reverseStrings(ids)
	start := (page - 1) * maxResults
	if start >= len(newestFirst) {
		return []string{}, nil
	}

	end := start + maxResults
	if end > len(newestFirst) {
		end = len(newestFirst)
	}

	return newestFirst[start:end], nil
}

func reverseStrings(ids []string) []string {
	reversed := make([]string, len(ids))
	for i, id := range ids {
		reversed[len(ids)-1-i] = id
	}

	return reversed
}

func (t *appstore) resolveVersionMetadata(acc Account, app App, guid string, platform Platform, ids []string) ([]ListedVersion, error) {
	if len(ids) == 0 {
		return []ListedVersion{}, nil
	}

	versions := make([]ListedVersion, len(ids))
	workers := versionMetadataWorkers
	if len(ids) < workers {
		workers = len(ids)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := make(chan int)
	var (
		wg       sync.WaitGroup
		fatalMu  sync.Mutex
		fatalErr error
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for index := range jobs {
				if ctx.Err() != nil {
					return
				}

				listed := ListedVersion{ExternalVersionID: ids[index]}
				metadata, metaErr := t.getVersionMetadata(acc, app, guid, ids[index], platform)
				if metaErr != nil {
					if errors.Is(metaErr, ErrPasswordTokenExpired) || errors.Is(metaErr, ErrLicenseRequired) {
						fatalMu.Lock()
						if fatalErr == nil {
							fatalErr = fmt.Errorf("resolve version %s: %w", ids[index], metaErr)
							cancel()
						}
						fatalMu.Unlock()

						return
					}

					listed.Error = fmt.Sprintf("resolve version %s: %s", ids[index], metaErr.Error())
					versions[index] = listed

					continue
				}

				listed.DisplayVersion = metadata.DisplayVersion
				listed.ReleaseDate = metadata.ReleaseDate
				versions[index] = listed
			}
		}()
	}

	go func() {
		defer close(jobs)

		for index := range ids {
			select {
			case <-ctx.Done():
				return
			case jobs <- index:
			}
		}
	}()

	wg.Wait()

	if fatalErr != nil {
		return nil, fatalErr
	}

	return versions, nil
}
