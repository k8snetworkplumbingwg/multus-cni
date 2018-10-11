# Validating admission webhook

## Building Docker image

From the root directory of Multus execute:
```
cd webhook
./build
```

## Deploying webhook application

Change working directory. From the root directory of Multus execute:
```
cd deployment/webhook
```

Create key and certificate pair and patch configuration-template.yaml file with base64-encoded certificate file. Run:
```
./certs.sh
```
*Note: Verify that Kubernetes controller manager has --cluster-signing-cert-file and --cluster-signing-key-file parameters set to paths to your CA keypair,
to make sure that Certificates API is enabled in order to generate certificate signed by cluster CA.
Script generates private key and certificate signing request, which is then pushed to the Kubernetes API server.
Then script approves that CSR and API server issues the certificate. Certificate is obtained from the API server and used to create a secret.
Script also patches `configuration-template.yaml` file with base64-encoded certificate and creates `configuration.yaml` file containing
Validating Webhook Configuration specification, which is deployed in one of the next steps.
More details about TLS certificates management in a cluster available [here](https://kubernetes.io/docs/tasks/tls/managing-tls-in-a-cluster/).*

Create service:
```
kubectl create -f service.yaml
```

Run deployment:
```
kubectl create -f deployment.yaml
```

Create Validating Webhook Configuration:
```
kubectl create -f configuration.yaml
```

## Verifying installation

Try to create invalid Network Attachment Definition resource:
```
cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: invalid-net-attach-def
spec:
  config: '{
    "invalid": "config"
  }'
EOF
```
Webhook should deny the request:
```
Error from server: error when creating "STDIN": admission webhook "multus-webhook.k8s.cni.cncf.io" denied the request: Invalid network config spec
```

Now, try to create correctly defined one:
```
cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: correct-net-attach-def
spec:
  config: '{
    "cniVersion": "0.3.0",
    "name": "a-bridge-network",
    "type": "bridge",
    "bridge": "br0",
    "isGateway": true,
    "ipam": {
      "type": "host-local",
      "subnet": "192.168.5.0/24",
      "dataDir": "/mnt/cluster-ipam"
    }
  }'
EOF
```
Resource should be allowed and created:
```
networkattachmentdefinition.k8s.cni.cncf.io/correct-net-attach-def created
```

## Troubleshooting
Webhook server prints a lot of debug messages that could help to find the root cause of an issue.
To display logs run:
```
kubectl logs -l app=multus-webhook
```
Example output showing logs for handling requests generated in the "Verifying installation section":
```
# kubectl logs multus-webhook-pod
2018-08-22T13:33:09Z [debug] Starting Multus webhook server
2018-08-22T13:33:32Z [debug] Validating network config spec: { "invalid": "config" }
2018-08-22T13:33:32Z [debug] Spec is not a valid network config: error parsing configuration list: no name. Trying to parse into config list
2018-08-22T13:33:32Z [debug] Spec is not a valid network config list: error parsing configuration: missing 'type'
2018-08-22T13:33:32Z [error] Invalid config: error parsing configuration: missing 'type'
2018-08-22T13:33:32Z [debug] Sending response to the API server
2018-08-22T13:35:29Z [debug] Validating network config spec: { "cniVersion": "0.3.0", "name": "a-bridge-network", "type": "bridge", "bridge": "br0", "isGateway": true, "ipam": { "type": "host-local", "subnet": "192.168.5.0/24", "dataDir": "/mnt/cluster-ipam" } }
2018-08-22T13:35:29Z [debug] Spec is not a valid network config: error parsing configuration list: no 'plugins' key. Trying to parse into config list
2018-08-22T13:35:29Z [debug] Network Attachment Defintion is valid. Admission Review request allowed
2018-08-22T13:35:29Z [debug] Sending response to the API server
```

