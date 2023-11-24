# Cluster Reflector Controller

Automatically connect your EKS, AKS and CAPI clusters to Weave Gitops Enterprise.

Weave GitOps Enterprise allows you to manage flux and other resources across multiple clusters.

Clusters can be added manually with the `gitops connect cluster` command, here we will use the Cluster Reflector Controller to automatically connect your clusters to Weave Gitops Enterprise.

## Provider support

At the moment the Cluster Reflector Controller supports the following providers:

- Azure AKS
- AWS EKS
- CAPI

## Design

Weave Gitops Enterprise uses a `GitopsCluster` CRD to represent a cluster. The `GitopsCluster` references a secret which contains the cluster credentials.

An `AutomatedClusterDiscovery` resource declares where to reflect the clusters from.

```yaml
apiVersion: clusters.weave.works/v1alpha1
kind: AutomatedClusterDiscovery
metadata:
  name: aks-cluster-discovery
  namespace: default
spec:
  type: aks
  interval: 10m
  aks:
    subscriptionID: subscription-123
```

The cluster reflector will list all the AKS clusters from this subscription and create a `GitopsCluster` and associated secret for each cluster in the same namespace as the `AutomatedClusterDiscovery` resource (`default` in this case).

## Configuration

### Type

Set the type of cluster to reflect.

```yaml
spec:
  type: "aks" # or `eks` or `capi`
```

### Interval

The interval at which the Cluster Reflector Controller will check for new clusters.

```yaml
spec:
  interval: 1h
```

### Common labels and annotations

Labels and annotations added to the reflected `GitopsCluster` and `Secret` resources

```yaml
spec:
  commonLabels:
    reflected-for: "dev"
  commonAnnotations:
    monitoring: "true"
```

### Reflecting AWS/Azure tags to Kubernetes Labels

Tags are a common way of organizing your clusters in AWS and Azure.

> [!NOTE]
> AWS uses the term "labels", here we'll use "tags" to refer to both AWS labels and Azure tags.

Any tags you add to your AKS/EKS clusters will be added as labels to the `GitopsCluster` resource. This is useful to tie into other components like the cluster bootstrap controller and the GitOpsSets controller. These controllers use labels to determine which clusters to act upon.

If your clusters have a lot of tags and reflecting them all is causing issues you can disabled this behavior with

```yaml
spec
  disableTags: true
```

## Azure

For `type: aks` reflection, we need to provide the subscription ID.

```yaml
type: "aks"
aks:
  subscriptionID: "12345678-1234-1234-1234-12
```

### Credentials

For azure we use a service principal to authenticate with Azure APIs.

The steps are roughly:

- Create a service principal with the `Contributor` role on the subscription
- Create a secret with the service principal credentials

  ```
  kubectl create secret generic provider-credentials \
      --namespace flux-system \
      --from-literal=AZURE_CLIENT_ID="client-123" \
      --from-literal=AZURE_CLIENT_SECRET="secret-123" \
      --from-literal=AZURE_TENANT_ID="tenant-id-123"
  ```

- Restart the cluster-reflector deployment which will pick up the new credentials from the `provider-credentials` secret.

> [!NOTE]
> The CAPI quickstart has a step by step guide on how to create a service principal and assign it the `Contributor` role on the subscription.
>
> - https://capz.sigs.k8s.io/topics/getting-started.html#prerequisites

## AWS

For `type: eks` reflection, we need to provide the region.

```yaml
type: "eks"
eks:
  region: "us-east-1"
```

### Credentials

If the Cluster Reflector Controller is hosted on the provider you're reflecting, we encourage you to use the providers authentication mechanism to provide credentials.

Use IAM roles for service accounts (IRSA) to associate an IAM role with the cluster-reflector deployment.

The steps are roughly:

- Create an IAM Policy that can list EKS clusters and get kubeconfigs.
- Create an IAM Role that can be _assumed_ by the cluster reflector (Ensure that the trust relationship of the role allows the EKS service to assume this role).
- Associate the IAM Role with the Cluster Reflector's Service account

  ```yaml
  cluster-reflector-controller:
    serviceAccount:
      annotations:
        eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/my-role
  ```

  OR if you have created a service account separately:

  ```yaml
  cluster-reflector-controller:
    serviceAccount:
      create: false
      name: my-service-account
  ```

## Connecting to other components

### Cluster Bootstrap Controller

The Cluster Bootstrap Controller allows you to run jobs against a `GitopsCluster` once it is Ready and has connectivity.

Its often used to run `flux bootstrap` on a new cluster to install flux and connect it to a Git Repository.

### GitOpsSets controller

GitOpsSets allow you to generate resources based on what `GitopsCluster`s are present.
