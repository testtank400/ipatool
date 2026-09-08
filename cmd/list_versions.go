package cmd

import (
	"errors"
	"time"

	"github.com/avast/retry-go"
	"github.com/majd/ipatool/v2/pkg/appstore"
	"github.com/spf13/cobra"
)

// nolint:wrapcheck
func ListVersionsCmd() *cobra.Command {
	var (
		appID           int64
		bundleID        string
		resolveMetadata bool
		page            int
		maxResults      int
	)

	cmd := &cobra.Command{
		Use:   "list-versions",
		Short: "List the available versions of an iOS app",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if page < 1 {
				return errors.New("page must be greater than 0")
			}

			if maxResults < 0 {
				return errors.New("max-results must not be negative")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if appID == 0 && bundleID == "" {
				return errors.New("either the app ID or the bundle identifier must be specified")
			}

			var lastErr error
			var acc appstore.Account

			return retry.Do(func() error {
				infoResult, err := dependencies.AppStore.AccountInfo()
				if err != nil {
					return err
				}

				acc = infoResult.Account

				if errors.Is(lastErr, appstore.ErrPasswordTokenExpired) {
					loginResult, err := dependencies.AppStore.Login(appstore.LoginInput{
						Email:    acc.Email,
						Password: acc.Password,
					})
					if err != nil {
						return err
					}

					acc = loginResult.Account
				}

				app := appstore.App{ID: appID}
				if bundleID != "" {
					lookupResult, err := dependencies.AppStore.Lookup(appstore.LookupInput{Account: acc, BundleID: bundleID})
					if err != nil {
						return err
					}

					app = lookupResult.App
				}

				out, err := dependencies.AppStore.ListVersions(appstore.ListVersionsInput{
					Account:         acc,
					App:             app,
					ResolveMetadata: resolveMetadata,
					Page:            page,
					MaxResults:      maxResults,
				})
				if err != nil {
					return err
				}

				event := dependencies.Logger.Log().
					Interface("externalVersionIdentifiers", out.ExternalVersionIdentifiers).
					Str("latestExternalVersionID", out.LatestExternalVersionID).
					Str("bundleID", app.BundleID).
					Int("count", len(out.ExternalVersionIdentifiers)).
					Int("totalCount", out.TotalCount).
					Int("page", out.Page).
					Bool("success", true)

				if resolveMetadata && cmd.Flag("format").Value.String() != "json" {
					for _, version := range out.Versions {
						versionEvent := dependencies.Logger.Log().Str("externalVersionID", version.ExternalVersionID)
						if version.Error != "" {
							versionEvent.Str("error", version.Error).Send()

							continue
						}

						versionEvent.
							Str("displayVersion", version.DisplayVersion).
							Time("releaseDate", version.ReleaseDate).
							Send()
					}

				}

				if resolveMetadata {
					event = event.Interface("versions", out.Versions)
				}

				event.Send()

				return nil
			},
				retry.LastErrorOnly(true),
				retry.DelayType(retry.FixedDelay),
				retry.Delay(time.Millisecond),
				retry.Attempts(2),
				retry.RetryIf(func(err error) bool {
					lastErr = err

					return errors.Is(err, appstore.ErrPasswordTokenExpired)
				}),
			)
		},
	}

	cmd.Flags().Int64VarP(&appID, "app-id", "i", 0, "ID of the target iOS app (required)")
	cmd.Flags().StringVarP(&bundleID, "bundle-identifier", "b", "", "The bundle identifier of the target iOS app (overrides the app ID)")
	cmd.Flags().BoolVar(&resolveMetadata, "resolve", false, "resolve each external version ID to a display version and release date (slow)")
	cmd.Flags().IntVarP(&page, "page", "p", 1, "page of versions to return (newest first when --max-results is set)")
	cmd.Flags().IntVarP(&maxResults, "max-results", "l", 0, "maximum number of versions to return (0 returns all)")

	return cmd
}
