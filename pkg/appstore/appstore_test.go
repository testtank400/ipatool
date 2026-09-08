package appstore

import (
	gohttp "net/http"
	"testing"

	"github.com/majd/ipatool/v2/pkg/http"
	"github.com/majd/ipatool/v2/pkg/util/machine"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

func TestAppStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "App Store Suite")
}

const (
	testDownloadMAC  = "aa:bb:cc:dd:ee:ff"
	testDownloadGUID = "AABBCCDDEEFF"
)

func expectActionSignerSetup(mockMachine *machine.MockMachine, mockBagClient *http.MockClient[bagResult]) {
	mockMachine.EXPECT().
		MacAddress().
		Return(testDownloadMAC, nil)
	mockBagClient.EXPECT().
		Send(gomock.Any()).
		Return(http.Result[bagResult]{
			StatusCode: gohttp.StatusOK,
			Data:       validBagResult(),
		}, nil)
}
