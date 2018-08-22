#!/bin/bash

# Copyright (c) 2018 Intel Corporation
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http:#www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# create temp dir to store intermediate files
tmp=$(mktemp -d)

# generate private key
echo "Generating private RSA key..."
openssl genrsa -out ${tmp}/webhook-key.pem 2048 >/dev/null 2>&1

# generate CSR
echo "Generating CSR configuration file..."
cat <<EOF >> ${tmp}/webhook.conf
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = multus-webhook-service
DNS.2 = multus-webhook-service.default
DNS.3 = multus-webhook-service.default.svc
EOF
openssl req -new -key ${tmp}/webhook-key.pem -subj "/CN=multus-webhook-service.default.svc" -out ${tmp}/server.csr -config ${tmp}/webhook.conf

# push CSR to Kubernetes API server
echo "Sending CSR to Kubernetes..."
csr_name="multus-webhook-service.default"
kubectl delete csr ${csr_name} >/dev/null 2>&1
cat <<EOF | kubectl create -f -
apiVersion: certificates.k8s.io/v1beta1
kind: CertificateSigningRequest
metadata:
  name: ${csr_name}
spec:
  request: $(cat ${tmp}/server.csr | base64 -w0)
  groups:
  - system:authenticated
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF

# approve certificate
echo "Approving CSR..."
kubectl certificate approve ${csr_name}

# wait for the cert to be issued
echo -n "Waiting for the certificate to be issued..."
cert=""
for sec in $(seq 15); do
  cert=$(kubectl get csr ${csr_name} -o jsonpath='{.status.certificate}')
  if [[ $cert != "" ]]; then
    echo -e "\nCertificate issued succesfully."
    echo $cert | base64 --decode > ${tmp}/webhook-cert.pem
    break
  fi
  echo -n "."; sleep 1
done
if [[ $cert == "" ]]; then
  echo -e "\nError: certificate not issued. Verify that the API for signing certificates is enabled."
  exit
fi

# create secret
echo "Creating secret..."
kubectl delete secret "multus-webhook-secret"
kubectl create secret generic --from-file=key.pem=${tmp}/webhook-key.pem --from-file=cert.pem=${tmp}/webhook-cert.pem "multus-webhook-secret"

# set cert in webhook configuration
echo "Patching configuration file with certificate..."
if [[ -f configuration-template.yaml ]]; then
  sed "s/__CERT__/${cert}/" configuration-template.yaml > configuration.yaml
  echo "File configuration.yaml patched."
else
  echo -e "Error: validating configuration template file 'configuration-template.yaml' is missing. Please update it with cert.pem value from the secret manually."
fi
