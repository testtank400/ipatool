package appstore

import (
	"fmt"
	"time"
)

type GetVersionMetadataInput struct {
	Account   Account
	App       App
	VersionID string
}

type GetVersionMetadataOutput struct {
	DisplayVersion string
	ReleaseDate    time.Time
}

func (t *appstore) GetVersionMetadata(input GetVersionMetadataInput) (GetVersionMetadataOutput, error) {
	signer, guid, err := t.newActionSigner()
	if err != nil {
		return GetVersionMetadataOutput{}, err
	}
	defer signer.Close()

	return t.getVersionMetadata(input.Account, input.App, guid, input.VersionID, signer)
}

func (t *appstore) getVersionMetadata(acc Account, app App, guid, versionID string, signer ActionSigner) (GetVersionMetadataOutput, error) {
	res, err := t.sendDownloadProduct(acc, app, guid, versionID, signer)
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
