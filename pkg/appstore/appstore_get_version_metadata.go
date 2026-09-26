package appstore

import (
	"fmt"
	"strings"
	"time"
)

type GetVersionMetadataInput struct {
	Account   Account
	App       App
	VersionID string
	Platform  Platform
}

type GetVersionMetadataOutput struct {
	DisplayVersion string
	ReleaseDate    time.Time
}

func (t *appstore) GetVersionMetadata(input GetVersionMetadataInput) (GetVersionMetadataOutput, error) {
	platform := input.Platform
	if platform == "" {
		platform = PlatformIPhone
	}

	switch platform {
	case PlatformIPhone, PlatformIPad, PlatformAppleTV, PlatformVisionOS, PlatformMacOS:
	default:
		return GetVersionMetadataOutput{}, fmt.Errorf("invalid platform %q", platform)
	}

	macAddr, err := t.machine.MacAddress()
	if err != nil {
		return GetVersionMetadataOutput{}, fmt.Errorf("failed to get mac address: %w", err)
	}

	guid := strings.ReplaceAll(strings.ToUpper(macAddr), ":", "")

	return t.getVersionMetadata(input.Account, input.App, guid, input.VersionID, platform)
}

func (t *appstore) getVersionMetadata(acc Account, app App, guid, versionID string, platform Platform) (GetVersionMetadataOutput, error) {
	res, _, err := t.sendDownloadProduct(acc, app, guid, versionID, platform)
	if err != nil {
		return GetVersionMetadataOutput{}, err
	}

	if err := interpretDownloadResult(res); err != nil {
		return GetVersionMetadataOutput{}, err
	}

	item := res.Data.Items[0]

	// Do not fall back to item.Metadata here. The App Store download API can
	// return stale version and release date values, so the IPA Info.plist is the
	// source of truth and failures should be visible to callers.
	metadata, err := t.readVersionMetadataFromIPA(item.URL)
	if err != nil {
		return GetVersionMetadataOutput{}, fmt.Errorf("failed to read version metadata: %w", err)
	}

	return GetVersionMetadataOutput(metadata), nil
}
