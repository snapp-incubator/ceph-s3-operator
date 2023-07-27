package rgwclient

import (
	"context"

	"github.com/ceph/go-ceph/rgw/admin"
)

type RgwClient interface {
	GetUser(ctx context.Context, user admin.User) (admin.User, error)
	CreateUser(ctx context.Context, user admin.User) (admin.User, error)
	GetQuota(ctx context.Context, quotaSpec admin.QuotaSpec) (admin.QuotaSpec, error)
	SetQuota(ctx context.Context, quotaSpec admin.QuotaSpec) error
	CreateSubuser(ctx context.Context, user admin.User, subuser admin.SubuserSpec) error
}
