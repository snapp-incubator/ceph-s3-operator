package rgwclient

import (
	"context"

	"github.com/ceph/go-ceph/rgw/admin"
)

type MockRgwClient struct{}

func NewMockRgwClient() *MockRgwClient {
	return &MockRgwClient{}
}

var _ RgwClient = &MockRgwClient{}

func (mrc *MockRgwClient) GetQuota(ctx context.Context, quotaSpec *admin.QuotaSpec) (*admin.QuotaSpec, error) {
	//TODO implement me
	panic("implement me")
}

func (mrc *MockRgwClient) SetQuota(ctx context.Context, quotaSpec *admin.QuotaSpec) error {
	//TODO implement me
	panic("implement me")
}

func (mrc *MockRgwClient) GetUser(ctx context.Context, user *admin.User) (*admin.User, error) {
	//TODO implement me
	panic("implement me")
}

func (mrc *MockRgwClient) CreateUser(ctx context.Context, user *admin.User) (*admin.User, error) {
	//TODO implement me
	panic("implement me")
}
