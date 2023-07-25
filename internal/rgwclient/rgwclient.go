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

func (r rgwClient) GetUser(ctx context.Context, user *admin.User) (*admin.User, error) {
	result, err := r.co.GetUser(ctx, *user)
	if err != nil {
		return nil, err
	}
	return &result, err
}

func (r rgwClient) ModifyUser(ctx context.Context, user *admin.User) (*admin.User, error) {
	result, err := r.co.ModifyUser(ctx, *user)
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
