package rgwclient

import (
	"context"

	"github.com/ceph/go-ceph/rgw/admin"
)

type RgwClient interface {
	GetUser(ctx context.Context, user *admin.User) (*admin.User, error)
	ModifyUser(ctx context.Context, user *admin.User) (*admin.User, error)
	CreateUser(ctx context.Context, user *admin.User) (*admin.User, error)
}
