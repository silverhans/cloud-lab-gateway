package openstack

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/cloud-lab-gateway/gateway/internal/domain/shared"
	"github.com/gophercloud/gophercloud/v2"
)

func translateErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	switch {
	case gophercloud.ResponseCodeIs(err, http.StatusNotFound):
		return fmt.Errorf("openstack: %s: %w", op, errors.Join(shared.ErrNotFound, err))
	case gophercloud.ResponseCodeIs(err, http.StatusUnauthorized),
		gophercloud.ResponseCodeIs(err, http.StatusRequestTimeout),
		gophercloud.ResponseCodeIs(err, http.StatusTooManyRequests),
		gophercloud.ResponseCodeIs(err, http.StatusInternalServerError),
		gophercloud.ResponseCodeIs(err, http.StatusBadGateway),
		gophercloud.ResponseCodeIs(err, http.StatusServiceUnavailable),
		gophercloud.ResponseCodeIs(err, http.StatusGatewayTimeout):
		return fmt.Errorf("openstack: %s: %w", op, errors.Join(shared.ErrCloudUnavailable, err))
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return fmt.Errorf("openstack: %s: %w", op, errors.Join(shared.ErrCloudUnavailable, err))
	}
	return fmt.Errorf("openstack: %s: %w", op, err)
}

func ignoreNotFound(err error) error {
	if err == nil || errors.Is(err, shared.ErrNotFound) || gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
		return nil
	}
	return err
}

func isNotFound(err error) bool {
	return errors.Is(err, shared.ErrNotFound) || gophercloud.ResponseCodeIs(err, http.StatusNotFound)
}

func isConflict(err error) bool {
	return gophercloud.ResponseCodeIs(err, http.StatusConflict)
}
