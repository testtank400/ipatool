package machine

import (
	"context"
	"errors"
	"fmt"

	"github.com/majd/ipatool/v2/internal/sap/assets"
	"github.com/majd/ipatool/v2/internal/sap/machimage"
)

type imageSpec struct {
	name string
	data []byte
	base uint64
}

type runtimeOptions struct {
	extraImages []imageSpec
	shims       shimOptions
}

func openRuntime(ctx context.Context, bundle assets.Bundle, options runtimeOptions) (*Machine, *machimage.Image, error) {
	if ctx == nil {
		return nil, nil, errors.New("SAP runtime context is nil")
	}

	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("open SAP runtime: %w", err)
	}

	if len(options.extraImages) == 0 {
		guest, err := Open(ctx, bundle)
		return guest, nil, err
	}

	return nil, nil, errors.New("StoreAgent extra images are not implemented")
}
