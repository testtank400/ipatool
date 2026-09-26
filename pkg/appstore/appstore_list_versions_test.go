package appstore

import (
	"errors"
	"fmt"
	gohttp "net/http"
	"net/http/httptest"
	"time"

	"github.com/majd/ipatool/v2/pkg/http"
	"github.com/majd/ipatool/v2/pkg/util/machine"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("AppStore (ListVersions)", func() {
	var (
		ctrl               *gomock.Controller
		mockBagClient      *http.MockClient[bagResult]
		mockDownloadClient *http.MockClient[downloadResult]
		mockPlatformClient *http.MockClient[platformVersionLookupResult]
		mockMachine        *machine.MockMachine
		as                 AppStore
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockBagClient = http.NewMockClient[bagResult](ctrl)
		mockDownloadClient = http.NewMockClient[downloadResult](ctrl)
		mockPlatformClient = http.NewMockClient[platformVersionLookupResult](ctrl)
		mockMachine = machine.NewMockMachine(ctrl)
		as = &appstore{
			bagClient:      mockBagClient,
			downloadClient: mockDownloadClient,
			platformClient: mockPlatformClient,
			machine:        mockMachine,
			httpClient:     http.NewClient[interface{}](http.Args{}),
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("pins the Mac offer before requesting a universal app's version history", func() {
		pages := http.NewMockClient[[]byte](ctrl)
		as.(*appstore).storefrontClient = pages
		mockMachine.EXPECT().MacAddress().Return("00:11:22:33:44:55", nil)
		gomock.InOrder(
			pages.EXPECT().Send(gomock.Any()).Do(func(req http.Request) {
				Expect(req.URL).To(Equal("https://apps.apple.com/de/app/id6472431552?platform=mac"))
			}).Return(http.Result[[]byte]{StatusCode: gohttp.StatusOK, Data: macVersionPage(karingMacConfiguration)}, nil),
			mockDownloadClient.EXPECT().Send(gomock.Any()).Do(func(req http.Request) {
				Expect(req.URL).To(ContainSubstring("volumeStoreDownloadProduct"))
				Expect(req.Payload.(*http.XMLPayload).Content).To(HaveKeyWithValue("externalVersionId", "876660716"))
			}).Return(http.Result[downloadResult]{StatusCode: gohttp.StatusOK, Data: downloadResult{Items: []downloadItemResult{{Metadata: map[string]interface{}{
				"softwareVersionExternalIdentifiers": []interface{}{uint64(876660700), uint64(876660716)},
				"softwareVersionExternalIdentifier":  uint64(876660716),
			}}}}}, nil),
		)
		out, err := as.ListVersions(ListVersionsInput{Account: Account{StoreFront: "143443-2,34"}, App: App{ID: 6472431552, BundleID: "com.nebula.karing"}, Platform: PlatformMacOS})
		Expect(err).ToNot(HaveOccurred())
		Expect(out.ExternalVersionIdentifiers).To(Equal([]string{"876660700", "876660716"}))
		Expect(out.LatestExternalVersionID).To(Equal("876660716"))
	})

	It("does not fall back to iOS when the Mac version cannot be resolved", func() {
		pages := http.NewMockClient[[]byte](ctrl)
		as.(*appstore).storefrontClient = pages
		mockMachine.EXPECT().MacAddress().Return("00:11:22:33:44:55", nil)
		pages.EXPECT().Send(gomock.Any()).Return(http.Result[[]byte]{StatusCode: gohttp.StatusOK, Data: macVersionPage(`{}`)}, nil)
		_, err := as.ListVersions(ListVersionsInput{Account: Account{StoreFront: "143443-2,34"}, App: App{ID: 42}, Platform: PlatformMacOS})
		Expect(err).To(MatchError(ContainSubstring("failed to resolve platform version")))
	})

	It("rejects unsupported platforms before making requests", func() {
		_, err := as.ListVersions(ListVersionsInput{Platform: PlatformUnknown})
		Expect(err).To(MatchError(`invalid platform "unknown"`))
	})

	When("fails to get MAC address", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("", errors.New(""))
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("request fails", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{}, errors.New(""))
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("request uses a custom pod", func() {
		const (
			testPod  = "42"
			testGUID = "001122334455"
		)

		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:11:22:33:44:55", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Do(func(req http.Request) {
					expectedURL := "https://p" + testPod + "-" + PrivateAppStoreAPIDomain + PrivateAppStoreAPIPathDownload + "?guid=" + testGUID
					Expect(req.URL).To(Equal(expectedURL))
				}).
				Return(http.Result[downloadResult]{}, errors.New(""))
		})

		It("sends the request to the pod-specific host", func() {
			_, err := as.ListVersions(ListVersionsInput{
				Account: Account{
					Pod: testPod,
				},
			})
			Expect(err).To(HaveOccurred())
		})
	})

	When("password token is expired", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						FailureType: FailureTypePasswordTokenExpired,
					},
				}, nil)
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("Sign In to the iTunes Store", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						FailureType: FailureTypeSignInRequired,
					},
				}, nil)
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("license is required", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						FailureType: FailureTypeLicenseNotFound,
					},
				}, nil)
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("store API returns error with customer message", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						FailureType:     "test-failure",
						CustomerMessage: "test error message",
					},
				}, nil)
		})

		It("returns error with customer message", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("test error message"))
		})
	})

	When("store API returns error without customer message", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						FailureType: "test-failure",
					},
				}, nil)
		})

		It("returns error with failure type", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("test-failure"))
		})
	})

	When("store API returns no items", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					StatusCode: gohttp.StatusOK,
					Data: downloadResult{
						Items: []downloadItemResult{},
					},
				}, nil)

			mockBagClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[bagResult]{
					StatusCode: gohttp.StatusOK,
					Data:       validBagResult(),
				}, nil)
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("version identifiers not found in metadata", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						Items: []downloadItemResult{
							{
								Metadata: map[string]interface{}{
									"someOtherKey": "someValue",
								},
							},
						},
					},
				}, nil)
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get version identifiers from item metadata"))
		})
	})

	When("latest version not found in metadata", func() {
		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						Items: []downloadItemResult{
							{
								Metadata: map[string]interface{}{
									"softwareVersionExternalIdentifiers": []interface{}{"12345678", "87654321"},
								},
							},
						},
					},
				}, nil)
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get latest version from item metadata"))
		})
	})

	It("lists the iOS history from a version-pinned fallback", func() {
		const latest = "890598805"
		bag := validBagResult()
		bag.URLBag.RedownloadEndpoint = testRedownloadEndpoint
		catalog := platformVersionLookupResult{Results: map[string]platformVersionLookupItem{
			"547702041": {Offers: []platformVersionLookupOffer{
				{Version: platformVersionLookupVersion{ExternalID: platformVersionExternalID(latest)}},
			}},
		}}

		mockMachine.EXPECT().MacAddress().Return("00:11:22:33:44:55", nil)
		gomock.InOrder(
			mockDownloadClient.EXPECT().Send(gomock.Any()).
				Return(http.Result[downloadResult]{StatusCode: gohttp.StatusOK}, nil),
			mockBagClient.EXPECT().Send(gomock.Any()).
				Return(http.Result[bagResult]{StatusCode: gohttp.StatusOK, Data: bag}, nil),
			mockPlatformClient.EXPECT().Send(gomock.Any()).
				Return(http.Result[platformVersionLookupResult]{StatusCode: gohttp.StatusOK, Data: catalog}, nil),
			mockDownloadClient.EXPECT().Send(gomock.Any()).
				Do(func(req http.Request) {
					Expect(req.Payload.(*http.XMLPayload).Content).To(HaveKeyWithValue("appExtVrsId", latest))
				}).
				Return(http.Result[downloadResult]{StatusCode: gohttp.StatusOK,
					Data: downloadResult{Items: []downloadItemResult{{Metadata: map[string]interface{}{
						"softwareVersionExternalIdentifiers": []interface{}{"9660833", latest},
						"softwareVersionExternalIdentifier":  latest,
					}}}}}, nil),
		)

		out, err := as.ListVersions(ListVersionsInput{
			Account: Account{StoreFront: "143441-1,34"},
			App:     App{ID: 547702041},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(out.ExternalVersionIdentifiers).To(Equal([]string{"9660833", latest}))
		Expect(out.LatestExternalVersionID).To(Equal(latest))
	})

	When("successfully lists versions", func() {
		const (
			testVersion1 = "12345678"
			testVersion2 = "87654321"
			testLatest   = "87654321"
		)

		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						Items: []downloadItemResult{
							{
								Metadata: map[string]interface{}{
									"softwareVersionExternalIdentifiers": []interface{}{testVersion1, testVersion2},
									"softwareVersionExternalIdentifier":  testLatest,
								},
							},
						},
					},
				}, nil)
		})

		It("returns versions", func() {
			out, err := as.ListVersions(ListVersionsInput{})
			Expect(err).ToNot(HaveOccurred())
			Expect(out.ExternalVersionIdentifiers).To(Equal([]string{testVersion1, testVersion2}))
			Expect(out.LatestExternalVersionID).To(Equal(testLatest))
		})
	})

	When("resolves display versions", func() {
		const (
			testVersion1 = "12345678"
			testVersion2 = "87654321"
			testLatest   = "87654321"
		)

		var (
			server1         *httptest.Server
			server2         *httptest.Server
			releaseDate1    time.Time
			releaseDate2    time.Time
			displayVersion1 = "1.0.0"
			displayVersion2 = "2.0.0"
		)

		listResult := http.Result[downloadResult]{
			Data: downloadResult{
				Items: []downloadItemResult{
					{
						Metadata: map[string]interface{}{
							"softwareVersionExternalIdentifiers": []interface{}{testVersion1, testVersion2},
							"softwareVersionExternalIdentifier":  testLatest,
						},
					},
				},
			},
		}

		BeforeEach(func() {
			releaseDate1 = time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)
			releaseDate2 = time.Date(2024, 4, 2, 12, 0, 0, 0, time.UTC)
			ipa1 := testIPA(displayVersion1, releaseDate1.Format(time.RFC3339), releaseDate1)
			ipa2 := testIPA(displayVersion2, releaseDate2.Format(time.RFC3339), releaseDate2)
			server1, _, _ = testIPAServer(ipa1)
			server2, _, _ = testIPAServer(ipa2)

			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)
		})

		AfterEach(func() {
			server1.Close()
			server2.Close()
		})

		expectVersionSend := func() {
			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				DoAndReturn(func(req http.Request) (http.Result[downloadResult], error) {
					payload, ok := req.Payload.(*http.XMLPayload)
					Expect(ok).To(BeTrue())

					switch payload.Content["externalVersionId"] {
					case nil, "":
						return listResult, nil
					case testVersion1:
						return http.Result[downloadResult]{
							Data: downloadResult{
								Items: []downloadItemResult{{URL: server1.URL}},
							},
						}, nil
					case testVersion2:
						return http.Result[downloadResult]{
							Data: downloadResult{
								Items: []downloadItemResult{{URL: server2.URL}},
							},
						}, nil
					default:
						return http.Result[downloadResult]{}, fmt.Errorf("unexpected version id %v", payload.Content["externalVersionId"])
					}
				}).
				AnyTimes()
		}

		It("returns display versions and release dates", func() {
			expectVersionSend()

			out, err := as.ListVersions(ListVersionsInput{ResolveMetadata: true})
			Expect(err).ToNot(HaveOccurred())
			Expect(out.ExternalVersionIdentifiers).To(Equal([]string{testVersion1, testVersion2}))
			Expect(out.TotalCount).To(Equal(2))
			Expect(out.Versions).To(HaveLen(2))
			Expect(out.Versions[0].ExternalVersionID).To(Equal(testVersion1))
			Expect(out.Versions[0].DisplayVersion).To(Equal(displayVersion1))
			Expect(out.Versions[0].ReleaseDate).To(Equal(releaseDate1))
			Expect(out.Versions[0].Error).To(BeEmpty())
			Expect(out.Versions[1].ExternalVersionID).To(Equal(testVersion2))
			Expect(out.Versions[1].DisplayVersion).To(Equal(displayVersion2))
			Expect(out.Versions[1].ReleaseDate).To(Equal(releaseDate2))
			Expect(out.Versions[1].Error).To(BeEmpty())
		})

		It("records per-version errors without failing the list", func() {
			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				DoAndReturn(func(req http.Request) (http.Result[downloadResult], error) {
					payload, ok := req.Payload.(*http.XMLPayload)
					Expect(ok).To(BeTrue())

					switch payload.Content["externalVersionId"] {
					case nil, "":
						return listResult, nil
					case testVersion1:
						return http.Result[downloadResult]{
							Data: downloadResult{
								Items: []downloadItemResult{{URL: server1.URL}},
							},
						}, nil
					case testVersion2:
						return http.Result[downloadResult]{
							Data: downloadResult{
								Items: []downloadItemResult{{URL: ""}},
							},
						}, nil
					default:
						return http.Result[downloadResult]{}, fmt.Errorf("unexpected version id %v", payload.Content["externalVersionId"])
					}
				}).
				AnyTimes()

			out, err := as.ListVersions(ListVersionsInput{ResolveMetadata: true})
			Expect(err).ToNot(HaveOccurred())
			Expect(out.Versions).To(HaveLen(2))
			Expect(out.Versions[0].DisplayVersion).To(Equal(displayVersion1))
			Expect(out.Versions[0].Error).To(BeEmpty())
			Expect(out.Versions[1].DisplayVersion).To(BeEmpty())
			Expect(out.Versions[1].Error).To(ContainSubstring("failed to read version metadata"))
		})

		It("aborts when the password token expires while resolving", func() {
			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				DoAndReturn(func(req http.Request) (http.Result[downloadResult], error) {
					payload, ok := req.Payload.(*http.XMLPayload)
					Expect(ok).To(BeTrue())
					if payload.Content["externalVersionId"] == nil || payload.Content["externalVersionId"] == "" {
						return listResult, nil
					}

					return http.Result[downloadResult]{
						Data: downloadResult{
							FailureType: FailureTypePasswordTokenExpired,
						},
					}, nil
				}).
				AnyTimes()

			out, err := as.ListVersions(ListVersionsInput{ResolveMetadata: true})
			Expect(err).To(MatchError(ErrPasswordTokenExpired))
			Expect(out.ExternalVersionIdentifiers).To(Equal([]string{testVersion1, testVersion2}))
			Expect(out.TotalCount).To(Equal(2))
		})

		It("resolves only the newest page when paginated", func() {
			expectVersionSend()

			out, err := as.ListVersions(ListVersionsInput{
				ResolveMetadata: true,
				Page:            1,
				MaxResults:      1,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(out.ExternalVersionIdentifiers).To(Equal([]string{testVersion2}))
			Expect(out.TotalCount).To(Equal(2))
			Expect(out.Page).To(Equal(1))
			Expect(out.Versions).To(HaveLen(1))
			Expect(out.Versions[0].ExternalVersionID).To(Equal(testVersion2))
			Expect(out.Versions[0].DisplayVersion).To(Equal(displayVersion2))
		})
	})

	When("paginates version identifiers", func() {
		const (
			v1     = "1"
			v2     = "2"
			v3     = "3"
			v4     = "4"
			v5     = "5"
			latest = "5"
		)

		BeforeEach(func() {
			mockMachine.EXPECT().
				MacAddress().
				Return("00:00:00:00:00:00", nil)
			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						Items: []downloadItemResult{
							{
								Metadata: map[string]interface{}{
									"softwareVersionExternalIdentifiers": []interface{}{v1, v2, v3, v4, v5},
									"softwareVersionExternalIdentifier":  latest,
								},
							},
						},
					},
				}, nil)
		})

		It("returns the newest page first", func() {
			out, err := as.ListVersions(ListVersionsInput{Page: 1, MaxResults: 2})
			Expect(err).ToNot(HaveOccurred())
			Expect(out.ExternalVersionIdentifiers).To(Equal([]string{v5, v4}))
			Expect(out.TotalCount).To(Equal(5))
			Expect(out.Page).To(Equal(1))
			Expect(out.Versions).To(BeEmpty())
		})

		It("returns the next older page", func() {
			out, err := as.ListVersions(ListVersionsInput{Page: 2, MaxResults: 2})
			Expect(err).ToNot(HaveOccurred())
			Expect(out.ExternalVersionIdentifiers).To(Equal([]string{v3, v2}))
			Expect(out.TotalCount).To(Equal(5))
			Expect(out.Page).To(Equal(2))
		})

		It("returns an empty page past the end", func() {
			out, err := as.ListVersions(ListVersionsInput{Page: 4, MaxResults: 2})
			Expect(err).ToNot(HaveOccurred())
			Expect(out.ExternalVersionIdentifiers).To(BeEmpty())
			Expect(out.TotalCount).To(Equal(5))
		})

		It("rejects an invalid page", func() {
			_, err := as.ListVersions(ListVersionsInput{Page: -1, MaxResults: 2})
			Expect(err.Error()).To(ContainSubstring("page must be greater than 0"))
		})
	})

})
