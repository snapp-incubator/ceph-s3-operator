package rgwclient

//import (
//	"context"
//
//	"github.com/ceph/go-ceph/rgw/admin"
//)
//
//type MockRgwClient struct {
//	quotaSpec *admin.QuotaSpec
//	user      *admin.User
//	err       error
//}
//
//func NewMockRgwClient(quotaSpec *admin.QuotaSpec, user *admin.User, err error) *MockRgwClient {
//	return &MockRgwClient{
//		quotaSpec: quotaSpec,
//		user:      user,
//		err:       err,
//	}
//}
//
//var _ RgwClient = &MockRgwClient{}
//
//func (mrc *MockRgwClient) GetQuota(context.Context, *admin.QuotaSpec) (*admin.QuotaSpec, error) {
//	if mrc.err != nil {
//		return nil, mrc.err
//	}
//	return mrc.quotaSpec, nil
//}
//
//func (mrc *MockRgwClient) SetQuota(context.Context, *admin.QuotaSpec) error {
//	return mrc.err
//}
//
//func (mrc *MockRgwClient) GetUser(context.Context, *admin.User) (*admin.User, error) {
//	if mrc.err != nil {
//		return nil, mrc.err
//	}
//	return mrc.user, nil
//}
//
//func (mrc *MockRgwClient) CreateUser(context.Context, *admin.User) (*admin.User, error) {
//	if mrc.err != nil {
//		return nil, mrc.err
//	}
//	return mrc.user, nil
//}
