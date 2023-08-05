package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type S3UserClassBased interface {
	GetS3UserClass() string
}

type S3ClassPredicate struct {
	S3UserClass string
}

func NewS3ClassPredicate(s3UserClass string) S3ClassPredicate {
	return S3ClassPredicate{
		S3UserClass: s3UserClass,
	}
}

func (scp S3ClassPredicate) MatchesS3UserClass(obj client.Object) bool {
	s3UserClassBased, ok := obj.(S3UserClassBased)
	if !ok {
		return false
	}
	return s3UserClassBased.GetS3UserClass() == scp.S3UserClass
}

func (scp S3ClassPredicate) Create(e event.CreateEvent) bool {
	return scp.MatchesS3UserClass(e.Object)
}

func (scp S3ClassPredicate) Delete(e event.DeleteEvent) bool {
	return scp.MatchesS3UserClass(e.Object)
}

func (scp S3ClassPredicate) Update(e event.UpdateEvent) bool {
	// Assuming immutable S3UserClass, there shouldn't be any difference checking ObjectOld or ObjectNew
	return scp.MatchesS3UserClass(e.ObjectNew)
}

func (scp S3ClassPredicate) Generic(e event.GenericEvent) bool {
	return scp.MatchesS3UserClass(e.Object)
}
