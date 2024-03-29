# Ceph S3 Operator

![License](https://img.shields.io/github/license/snapp-incubator/ceph-s3-operator)
![Test](https://github.com/snapp-incubator/ceph-s3-operator/actions/workflows/checks.yaml/badge.svg?branch=main)
![Release](https://github.com/snapp-incubator/ceph-s3-operator/actions/workflows/build-release.yaml/badge.svg)
![Tag](https://img.shields.io/github/v/tag/snapp-incubator/ceph-s3-operator?&logo=git)

## Introduction

The Ceph S3 Operator, an open-source endeavor, is crafted to streamline the management of S3 users and buckets within a Ceph cluster environment. It enhances efficiency and simplifies processes, rendering S3 usage on Ceph clusters more straightforward and user-friendly.

## Features

- S3 User Management
- Bucket Management
- Subuser Support
- Bucket policy Support
- Quota Management
- Webhook Integration
- E2E Testing
- Helm Chart and OLM Support

## Installation

### Prerequisites

- Kubernetes v1.23.0+
- Ceph v14.2.10+
    > Note: prior Ceph versions [don't support the subuser bucket policy](https://github.com/ceph/ceph/pull/33714). Nevertheless, other features are expected to work properly within those earlier releases.
- ClusterResourceQuota CRD: `kubectl apply -f config/external-crd`

### Using OLM

You can find the operator on [OperatorHub](https://operatorhub.io/operator/ceph-s3-operator) and install it using OLM.

### Using Helm

Deploy using Helm (version 3.8.0 or later), which supports OCI charts. To use the helm chart, edit the `values.yaml` file and set `controllerManagerConfig.configYaml` to your Ceph cluster configuration like [secret.yaml](config/manager/secret.yaml).

```bash
helm upgrade --install ceph-s3-operator oci://ghcr.io/snapp-incubator/ceph-s3-operator/helm-charts/ceph-s3-operator --version v0.3.7
```

### Using Makefile

Deploy using a simple command:

```bash
make deploy
```

## Usage and Documentation

- CRD Examples: Located in the [samples](config/samples) folder.
- Detailed Documentation: Available on the [wiki](https://github.com/snapp-incubator/ceph-s3-operator/wiki).
- Design and Decision Insights: Refer to our [design doc](docs/DESIGN.md) for an in-depth understanding.

## Versioning and Release

A new docker image, bundle and helm chart will be created each time a tag starting with `v` is pushed to the main branch.

## Development

We follow [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder/blob/master/DESIGN.md#development) development principles, Specifically about testing in an environment similar to the real world and avoiding mocks as much as
possible.

For example, we don't mock RGW API. Instead, we use a similar approach to
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

The chart will be created/updated in `charts/ceph-s3-operator` path

### Run locally

If you want to test the operator on your local environment, run the below instructions:

First setup the local Ceph cluster:

```shell
make setup-dev-env
```

Then run the operator either with or without webhook:

```shell
make run  # Without webhook
make run-with-webhook # With webhook
```

At the end, you can tear down the operator and the Ceph cluster:

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

## Contributing

Contributions are warmly welcomed. Feel free to submit issues or pull requests.

## License

This project is licensed under the [GPL 3.0](https://github.com/snapp-incubator/ceph-s3-operator/blob/main/LICENSE).
