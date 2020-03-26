# cert-manager-webhook-safedns

### Installing

The webhook can be installed with Helm as below:

* `helm repo add ukfast https://ukfast.github.io/helm-charts`
* `helm repo update`
* `helm install cert-manager-webhook-safedns ukfast/cert-manager-webhook-safedns`

> :warning: Installing via Helm currently requires Kubernetes `v1.17.0` and above (due to missing permissions in `extension-apiserver-authentication-reader`).
  this can be worked around by either creating a new role/role binding, or adding the following permissions to the `extension-apiserver-authentication-reader` role:

```
- apiGroups:
  - ""
  resourceNames:
  - extension-apiserver-authentication
  resources:
  - configmaps
  verbs:
  - list
- apiGroups:
  - ""
  resourceNames:
  - extension-apiserver-authentication
  resources:
  - configmaps
  verbs:
  - watch
```

### Getting started

The SafeDNS webhook requires an API key with read/write permissions. This should be obtained via MyUKFast before continuing

First, we'll create a `Secret` containing our API key:

```
kubectl create secret generic safedns-api-key --from-literal=api_key=<API_KEY>
```

Next, we'll configure a LetsEncrypt `Issuer` using the SafeDNS `solver`:

```
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1alpha2
kind: Issuer
metadata:
  name: letsencrypt-prod-safedns
spec:
  acme:
    email: admin@example.com
    privateKeySecretRef:
      name: letsencrypt-prod
    server: https://acme-v02.api.letsencrypt.org/directory
    solvers:
    - dns01:
        webhook:
          solverName: safedns
          groupName: acme.k8s.ukfast.io
          config:
            apiKeySecretRef:
              name: safedns-api-key
              key: api_key
EOF
```

Finally, we'll create our certificate:

```
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1alpha2
kind: Certificate
metadata:
  name: wildcard-example-com
spec:
  dnsNames:
  - '*.example.com'
  issuerRef:
    kind: Issuer
    name: letsencrypt-prod-safedns
  secretName: wildcard-example-com-tls
EOF
```

### Running the test suite

`apikey.yml` should first be created in `testdata/safedns` (example at `testdata/safedns/apikey.sample.yml`) before executing the test suite.
These tests require several binaries, which can be downloaded via `scripts/fetch-test-binaries.sh`

The test suite is executed via `go test` as below:

```bash
$ TEST_ZONE_NAME=example.com. go test .
```
