# This file creates SSL certicates via mkcert and puts the caBundle pem in an env file.

# export the CAROOT env variable, used by the mkcert tool to generate CA and certs
export CAROOT=/tmp/k8s-webhook-server/serving-certs

# then, install a new CA
# the following command will create 2 files rootCA.pem and rootCA-key.pem
mkcert -install

# then, generate SSL certificates
# here, we're creating certificates valid for different possible docker host addresses.
# and put them inside the certs/tls.crt and certs/tls.key files (by default the operator/webhook will look for certificates with this naming convention)
mkcert -cert-file=$CAROOT/tls.crt -key-file=$CAROOT/tls.key host.minikube.internal 192.168.64.1 host.docker.internal 172.17.0.1
CA_BUNDLE=$(cat $CAROOT/rootCA.pem | base64 -w 0)

echo CA_BUNDLE=$CA_BUNDLE > environment-properties.env
