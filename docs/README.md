# Cluster Reflector Controller

Automatically connect your EKS, AKS and CAPI clusters to Weave Gitops Enterprise.

Weave GitOps Enterprise allows you to manage flux and other resources across multiple clusters.

Clusters can be added manually with the `gitops connect cluster` command, however with many clusters this can become expensive. Cluster Reflector Controller will add and remove clusters automatically based on the `AutomatedClusterDiscovery` resource.

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

For Azure AKS we use a [service principal](https://learn.microsoft.com/en-us/azure/aks/kubernetes-service-principal?tabs=azure-cli) to authenticate with Azure [APIs](https://learn.microsoft.com/en-us/rest/api/aks/managed-clusters/list?view=rest-aks-2023-08-01&tabs=HTTP).

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

EXAMPLE

### GitOpsSets controller

GitOpsSets allow you to generate resources based on what `GitopsCluster`s are present.

If we tag our AKS/EKS cluster with `wego-admin-rbac: enabled` in the Azure portal or AWS Console, then the Cluster Reflector will create a GitopsCluster with that label.

We can then create a GitOpsSet that will generate a Kustomization for each cluster with the label `wego-admin-rbac: enabled`.

In this example the kustomization loads a kustomization from a `clusters/bases` directory. This is often where we keep common RBAC / NetworkingPolicy.

```yaml
apiVersion: templates.weave.works/v1alpha1
kind: GitOpsSet
metadata:
  name: gitopsset-cluster-wego-admin-rbac
  namespace: default
spec:
  generators:
    - cluster:
        selector:
          matchLabels:
            wego-admin-rbac: enabled
  templates:
    - content:
        apiVersion: kustomize.toolkit.fluxcd.io/v1
        kind: Kustomization
        metadata:
          name: "{{ .Element.ClusterName }}-wego-admin-rbac"
          namespace: default
        spec:
          interval: 10m0s
          kubeConfig:
            secretRef:
              name: "{{ .Element.ClusterName }}-kubeconfig"
          sourceRef:
            kind: GitRepository
            name: flux-system
            namespace: flux-system
          path: ./clusters/bases
          prune: true
```
