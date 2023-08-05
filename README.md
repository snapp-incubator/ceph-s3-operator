# S3 Operator Helm Chart

## Usage

[Helm](https://helm.sh) must be installed to use the charts. Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

helm repo add s3-operator https://snapp-incubator.github.io/s3-operator

If you had already added this repo earlier, run `helm repo update` to retrieve
the latest versions of the packages. You can then run `helm search repo
s3-operator` to see the charts.

To install the s3-operator chart:

    helm install my-s3-operator s3-operator/s3-operator

To uninstall the chart:

    helm delete my-s3-operator
