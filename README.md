# S3 Operator

## Introduction

At Snapp! we are using Ceph object storage to have S3 for users, this operator is here
to make working with S3 easier and more fun.

## Objects

Following object is defined for each namespace:

```yaml
apiVersion: s3.snappcloud.io/v1alpha
Kind: S3UserClaim
metadata:
  name: myuser
  namespace: dispatching-test
spec:
  s3ClassName: (optional) have default value
  readOnlySecret: (optional)
  adminSecret: (required)
status:
  quota: (max_buckets, max_size, max_objects)
```

and this object is cluster scoped:

```yaml
apiVersion: s3.snappcloud.io/v1alpha
Kind: S3User
metadata:
  name: myuser
spec:
  s3ClassName:
  claimPolicy: Delete / Retain
  claimRef:
    apiVersion: v1
    kind: S3UserClaim
    name: myuser
    namespace: baly-ode-001
    resourceVersion: "267741823"
    uid: ff1eddc9-fb16-4762-ba43-f193ed23b92d
  quota: (max_buckets, max_size, max_objects)
status:
```

## Development

We follow [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder/blob/master/DESIGN.md#development) developement
principles, Specifically about testing in an environment similar to the real world and avoiding mocks as much as
possible.

For example, we don't mock RGW API. Instead, we use a simliar approach to
what [go-ceph](https://github.com/ceph/go-ceph/) does.

### Building the testing image

```shell
TESTING_IMAGE_TAG=<desired_tag> make build-testing-image
```

Don't forget to update the tag in Makefile!

### Building the helm chart

We use [helmify](https://github.com/arttor/helmify) to generate Helm chart from kustomize rendered manifests. To update
the chart run:

```shell
make helm
```

The chart will be created/updated in `deploy/charts/s3-operator` path
