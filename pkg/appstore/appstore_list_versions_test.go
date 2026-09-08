package appstore

import (
	"errors"
	"fmt"
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
		mockDownloadClient *http.MockClient[downloadResult]
		mockBagClient      *http.MockClient[bagResult]
		mockMachine        *machine.MockMachine
		signer             *stubActionSigner
		as                 AppStore
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockDownloadClient = http.NewMockClient[downloadResult](ctrl)
		mockBagClient = http.NewMockClient[bagResult](ctrl)
		mockMachine = machine.NewMockMachine(ctrl)
		signer = &stubActionSigner{}
		as = &appstore{
			downloadClient: mockDownloadClient,
			bagClient:      mockBagClient,
			machine:        mockMachine,
			httpClient:     http.NewClient[interface{}](http.Args{}),
			actionSignerFactory: func(config SAPConfig, machineID []byte) (ActionSigner, error) {
				Expect(config).To(Equal(validSAPConfig()))
				Expect(machineID).To(Equal([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}))

				return signer, nil
			},
		}
	})

	AfterEach(func() {
		ctrl.Finish()
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
			expectActionSignerSetup(mockMachine, mockBagClient)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{}, errors.New("")).
				Times(4)
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("request uses a custom pod", func() {
		const testPod = "42"

		BeforeEach(func() {
			expectActionSignerSetup(mockMachine, mockBagClient)

			calls := 0
			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				DoAndReturn(func(req http.Request) (http.Result[downloadResult], error) {
					if calls == 0 {
						Expect(req.URL).To(Equal("https://p" + testPod + "-" + PrivateAppStoreAPIDomain + "/WebObjects/MZFinance.woa/wa/redownloadProduct?guid=" + testDownloadGUID))
						Expect(req.ActionSigner).ToNot(BeNil())
					}
					calls++

					return http.Result[downloadResult]{}, errors.New("")
				}).
				Times(4)
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
			expectActionSignerSetup(mockMachine, mockBagClient)

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
			expectActionSignerSetup(mockMachine, mockBagClient)

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
			expectActionSignerSetup(mockMachine, mockBagClient)

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
			expectActionSignerSetup(mockMachine, mockBagClient)

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
			expectActionSignerSetup(mockMachine, mockBagClient)

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
			expectActionSignerSetup(mockMachine, mockBagClient)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Return(http.Result[downloadResult]{
					Data: downloadResult{
						Items: []downloadItemResult{},
					},
				}, nil).
				Times(4)
		})

		It("returns error", func() {
			_, err := as.ListVersions(ListVersionsInput{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("the first download endpoint returns no items", func() {
		const (
			testVersion1 = "12345678"
			testVersion2 = "87654321"
			testLatest   = "87654321"
		)

		BeforeEach(func() {
			expectActionSignerSetup(mockMachine, mockBagClient)

			gomock.InOrder(
				mockDownloadClient.EXPECT().
					Send(gomock.Any()).
					Return(http.Result[downloadResult]{
						Data: downloadResult{
							Items: []downloadItemResult{},
						},
					}, nil),
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
					}, nil),
			)
		})

		It("uses the next endpoint", func() {
			out, err := as.ListVersions(ListVersionsInput{})
			Expect(err).ToNot(HaveOccurred())
			Expect(out.ExternalVersionIdentifiers).To(Equal([]string{testVersion1, testVersion2}))
			Expect(out.LatestExternalVersionID).To(Equal(testLatest))
		})
	})

	When("version identifiers not found in metadata", func() {
		BeforeEach(func() {
			expectActionSignerSetup(mockMachine, mockBagClient)

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
			expectActionSignerSetup(mockMachine, mockBagClient)

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

	When("successfully lists versions", func() {
		const (
			testVersion1 = "12345678"
			testVersion2 = "87654321"
			testLatest   = "87654321"
		)

		account := Account{
			DirectoryServicesID: "test-dsid",
			StoreFront:          "143441",
			PasswordToken:       "password-token",
		}

		BeforeEach(func() {
			expectActionSignerSetup(mockMachine, mockBagClient)

			mockDownloadClient.EXPECT().
				Send(gomock.Any()).
				Do(func(req http.Request) {
					Expect(req.ActionSigner).To(BeIdenticalTo(signer))
					Expect(req.Headers).To(HaveKeyWithValue("X-Apple-Store-Front", account.StoreFront))
					Expect(req.Headers).To(HaveKeyWithValue("X-Token", account.PasswordToken))
					Expect(req.Headers).To(HaveKeyWithValue("iCloud-DSID", account.DirectoryServicesID))
					Expect(req.Headers).To(HaveKeyWithValue("X-Dsid", account.DirectoryServicesID))
				}).
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
			out, err := as.ListVersions(ListVersionsInput{Account: account})
			Expect(err).ToNot(HaveOccurred())
			Expect(out.ExternalVersionIdentifiers).To(Equal([]string{testVersion1, testVersion2}))
			Expect(out.LatestExternalVersionID).To(Equal(testLatest))
			Expect(out.Versions).To(BeEmpty())
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

			expectActionSignerSetup(mockMachine, mockBagClient)
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

			_, err := as.ListVersions(ListVersionsInput{ResolveMetadata: true})
			Expect(err).To(Equal(ErrPasswordTokenExpired))
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
			expectActionSignerSetup(mockMachine, mockBagClient)
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
			Expect(err).To(MatchError("page must be greater than 0"))
		})
	})
})
