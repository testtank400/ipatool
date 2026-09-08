package appstore

import (
	"time"

	"github.com/rs/zerolog"
)

type App struct {
	ID           int64     `json:"trackId,omitempty"`
	BundleID     string    `json:"bundleId,omitempty"`
	Name         string    `json:"trackName,omitempty"`
	Version      string    `json:"version,omitempty"`
	Price        float64   `json:"price,omitempty"`
	PurchaseDate time.Time `json:"purchaseDate,omitzero"`
}

type VersionHistoryInfo struct {
	App                App
	LatestVersion      string
	VersionIdentifiers []string
}

type VersionDetails struct {
	VersionID     string
	VersionString string
	Success       bool
	Error         string
}

type ListedVersion struct {
	ExternalVersionID string    `json:"externalVersionID"`
	DisplayVersion    string    `json:"displayVersion,omitempty"`
	ReleaseDate       time.Time `json:"releaseDate,omitzero"`
	Error             string    `json:"error,omitempty"`
}

func (v ListedVersion) MarshalZerologObject(event *zerolog.Event) {
	event.Str("externalVersionID", v.ExternalVersionID)

	if v.DisplayVersion != "" {
		event.Str("displayVersion", v.DisplayVersion)
	}

	if !v.ReleaseDate.IsZero() {
		event.Time("releaseDate", v.ReleaseDate)
	}

	if v.Error != "" {
		event.Str("error", v.Error)
	}
}

type Apps []App

func (apps Apps) MarshalZerologArray(a *zerolog.Array) {
	for _, app := range apps {
		a.Object(app)
	}
}

func (a App) MarshalZerologObject(event *zerolog.Event) {
	event.
		Int64("id", a.ID).
		Str("bundleID", a.BundleID).
		Str("name", a.Name).
		Str("version", a.Version).
		Float64("price", a.Price)

	if !a.PurchaseDate.IsZero() {
		event.Time("purchaseDate", a.PurchaseDate)
	}
}
