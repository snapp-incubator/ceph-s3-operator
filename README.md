# S3 Operator

## Introduction

The S3 Operator, an open-source endeavor, is crafted to streamline the management of S3 users and buckets within a Ceph cluster environment. It enhances efficiency and simplifies processes, rendering S3 usage on Ceph clusters more straightforward and user-friendly.

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

### Using Makefile

Deploy using a simple command:

```bash
make deploy
```

### Using Helm

Deploy using Helm (version 3.8.0 or later), which supports OCI charts. To use the helm chart, edit the `values.yaml` file and set `controllerManagerConfig.configYaml` to your Ceph cluster configuration like [secret.yaml](config/manager/secret.yaml).

```bash
helm upgrade -i s3-operator oci://ghcr.io/s3-operator/helm-charts/s3-operator
```

### Using OLM

All the operator releases are bundled and pushed to the [Snappcloud hub](https://github.com/snapp-incubator/snappcloud-hub) which is a hub for the catalog sources. Install using Operator Lifecycle Manager (OLM) by following these steps:

1. Install [snappcloud hub catalog-source](https://github.com/snapp-incubator/snappcloud-hub/blob/main/catalog-source.yml)

2. Override the `s3-operator-controller-manager-config-override` with your operator configuration.
3. Apply the subscription manifest as shown below:

    ```yaml
    apiVersion: operators.coreos.com/v1alpha1
    kind: Subscription
    metadata:
    name: s3-operator
    namespace: operators
    spec:
    channel: stable-v0
    installPlanApproval: Automatic
    name: s3-operator
    source: snappcloud-hub-catalog
    sourceNamespace: openshift-marketplace
    config:
        resources:
        limits:
            cpu: 2
            memory: 2Gi
        requests:
            cpu: 1
            memory: 1Gi
        volumes:
        - name: config
        secret:
            items:
            - key: config.yaml
            path: config.yaml
            secretName: s3-operator-controller-manager-config-override
        volumeMounts:
        - mountPath: /s3-operator/config/
        name: config
    ```

## Usage and Documentation

- CRD Examples: Located in the [samples](config/samples) folder.
- Detailed Documentation: Available on the [wiki](https://github.com/snapp-incubator/s3-operator/wiki).
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

The chart will be created/updated in `charts/s3-operator` path

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

This project is licensed under the [Apache License 2.0](https://github.com/snapp-incubator/s3-operator/blob/main/LICENSE).
