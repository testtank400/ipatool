package appstore

import (
	"context"
	"fmt"

	"github.com/majd/ipatool/v2/internal/sap"
	"github.com/majd/ipatool/v2/pkg/http"
)

type SAPConfig struct {
	AuthEndpoint   string
	SetupURL       string
	CertificateURL string
	Version        uint32
}

type ActionSigner interface {
	http.ActionSigner
	Close() error
}

type ActionSignerFactory func(config SAPConfig, machineID []byte) (ActionSigner, error)

func defaultActionSignerFactory(config SAPConfig, machineID []byte) (ActionSigner, error) {
	signer, err := sap.NewSigner(context.Background(), sap.Config{
		SetupURL:       config.SetupURL,
		CertificateURL: config.CertificateURL,
		Version:        config.Version,
		HardwareID:     machineID,
	})
	if err != nil {
		return nil, fmt.Errorf("create SAP signer: %w", err)
	}

	return signer, nil
}

func (t *appstore) newActionSigner() (ActionSigner, string, error) {
	macAddr, err := t.machine.MacAddress()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get mac address: %w", err)
	}

	guid, machineID, err := machineIdentity(macAddr)
	if err != nil {
		return nil, "", err
	}

	if t.actionSignerFactory == nil {
		return nil, "", fmt.Errorf("SAP action signer is not configured")
	}

	bag, err := t.bag(guid)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get bag: %w", err)
	}

	signer, err := t.actionSignerFactory(bag.SAPConfig, machineID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to initialize SAP action signer: %w", err)
	}

	if signer == nil {
		return nil, "", fmt.Errorf("SAP action signer factory returned nil")
	}

	return signer, guid, nil
}
