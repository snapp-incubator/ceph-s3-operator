# S3 Operator

## Introduction

At Snapp! we are using Ceph object storage to have S3 for users, this operator is here
to make working with S3 easier and more fun.

## Design

For the detailed discription of the design and decisions, pelease see our [design-doc](docs/DESIGN.md).

## Versioning

A new docker image will be created each time a tag starting with `v` is pushed to the main branch.

For Helm charts, there's a relase job that will create a new
release [when a change is detected](https://helm.sh/docs/howto/chart_releaser_action/#github-actions-workflow) in
the `deploy/charts` directory.

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

### Run locally

If you want to test the operator on your local environment, run the below instruction:

First setup the local ceph cluster:

```shell
make setup-dev-env
```

Then run the operator either with or without webhook:

```shell
make run  # Without webhook
make run-with-webhook # With webhook
```

At the end you can tear-down the operator and the ceph cluster:

```shell
make teardown-operator teardown-dev-env
```

## Test

To test the project via the operator-sdk `envtest`:

```shell
make test
```

And to run the e2e tests with KUTTL performing the tests on a KIND cluster:

```shell
kubectl-kuttl test
```
