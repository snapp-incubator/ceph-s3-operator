# S3 Operator

## Introduction

The S3 Operator is an open-source project designed to facilitate the management of S3 users and buckets in a Ceph cluster environment. It simplifies operations, making working with S3 on Ceph clusters easier and more efficient.

## Features

- Create/Remove a s3User
- Create/Remove a bucket with retain policy
- Subuser support
- Bucket policy support
- Quota Management
- Webhook Integration
- E2E Testing
- Helm Chart and OLM support

## Installation

### Using Makefile

```bash
make deploy
```

### Using Helm

This is an OCI helm chart, helm started supporting OCI in version 3.8.0. To use the helm chart, edit the `values.yaml` file and set `controllerManagerConfig.configYaml` to your Ceph cluster configuration like [secret.yaml](config/manager/secret.yaml).

```bash
helm upgrade -i s3-operator oci://ghcr.io/s3-operator/helm-charts/s3-operator
```

### Using OLM

All the operator releases are bundled and pushed to the [snappcloud-hub](https://github.com/snapp-incubator/snappcloud-hub) which is a hub for the catalog sources. To install the operator in an OLM way:
1. Install [snappcloud hub catalog-source](https://github.com/snapp-incubator/snappcloud-hub/blob/main/catalog-source.yml)

2. Provide the operator `configuration` in `s3-operator-controller-manager-config-override`
3. Apply the below subscription manifest:

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

## Usage

You can find the CRD examples in the [samples](config/samples) folder. Additionally, you can find a rich documentation about the CRD details in the [wiki](https://github.com/snapp-incubator/s3-operator/wiki) page.

For the detailed discription of the design and decisions, pelease see our [design-doc](docs/DESIGN.md).

## Versioning

A new docker image, bundle and helm chart will be created each time a tag starting with `v` is pushed to the main branch.

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

The chart will be created/updated in `charts/s3-operator` path

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

## Contributing

Contributions to the S3 Operator are welcome. Feel free to submit an issue or PR.

## License

This project is licensed under the [Apache License 2.0](https://github.com/snapp-incubator/s3-operator/blob/main/LICENSE).
