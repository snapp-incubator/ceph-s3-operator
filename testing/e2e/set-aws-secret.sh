#!/bin/bash
# This file gets the s3user credentials from the secret and sets as the aws-cli credentials.
# Replace with your Kubernetes secret name and profile name
SECRET_NAME="s3-sample-admin-secret"
PROFILE_NAME="ceph-test"
REGION="us-east-1"

# Get the access key and secret key from Kubernetes secret
ACCESS_KEY=$(kubectl get secret $SECRET_NAME -n s3-test -o jsonpath="{.data.accessKey}" | base64 --decode)
SECRET_ACCESS_KEY=$(kubectl get secret $SECRET_NAME -n s3-test -o jsonpath="{.data.secretKey}" | base64 --decode)

# Update the existing profile or create a new one
aws configure set aws_access_key_id $ACCESS_KEY --profile $PROFILE_NAME
aws configure set aws_secret_access_key $SECRET_ACCESS_KEY --profile $PROFILE_NAME
aws configure set region $REGION --profile $PROFILE_NAME