package rgwclient

import (
	"context"

	"github.com/ceph/go-ceph/rgw/admin"
)

type rgwClient struct {
	co *admin.API
}

func NewRgwClient(co *admin.API) RgwClient {
	return &rgwClient{
		co: co,
	}
}

var _ RgwClient = &MockRgwClient{}

func (r rgwClient) GetUser(ctx context.Context, user *admin.User) (*admin.User, error) {
	result, err := r.co.GetUser(ctx, *user)
	if err != nil {
		return nil, err
	}
	return &result, err
}

func (r rgwClient) CreateUser(ctx context.Context, user *admin.User) (*admin.User, error) {
	result, err := r.co.CreateUser(ctx, *user)
	if err != nil {
		return nil, err
	}
	return &result, err
}

// GetQuota retrieves the user quota. The quotaSpec arg is only used for settings the UID in the request.
func (r rgwClient) GetQuota(ctx context.Context, quotaSpec *admin.QuotaSpec) (*admin.QuotaSpec, error) {
	q, err := r.co.GetUserQuota(ctx, *quotaSpec)
	return &q, err
}

func (r rgwClient) SetQuota(ctx context.Context, quotaSpec *admin.QuotaSpec) error {
	return r.co.SetUserQuota(ctx, *quotaSpec)
}
