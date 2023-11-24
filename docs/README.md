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

The cluster reflector will list all the AKS clusters from this subscription and create a `GitopsCluster` and associated secret for each cluster.

## Credentials

If the Cluster Reflector Controller is hosted on the provider you're reflecting, we encourage you to use the providers authentication mechanism to provide credentials.

### AWS

Use IAM roles for service accounts (IRSA) to associate an IAM role with the cluster-reflector deployment.

The steps are roughly:

- Create an IAM Policy that can list EKS clusters
- Create an IAM Role that can be _assumed_ by the cluster reflector (Ensure that the trust relationship of the role allows the EKS service to assume this role).
- Associate the IAM Role with the Cluster Reflector' Service account

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

### Azure

Azure uses a service principal to authenticate with Azure.

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

## Configration

### Adding labels

Labels are a common way of organizing your clusters in AWS and Azure.

> [!NOTE]
> Azure uses the term "tags", we'll use the term "labels" to cover both.

Any labels you add to your clusters will be added to the `GitopsCluster` resource. This is useful to tie into other components like the cluster bootstrap controller and the GitOpsSets controller.

If your clusters have a lot of labels and reflecting them all is causing issues you can disabled this behavior with `spec.disableTags: true`.

## Connecting to other components

### Cluster Bootstrap Controller

The Cluster Bootstrap Controller allows you to run jobs against a `GitopsCluster` once it is Ready and has connectivity.

Its often used to run `flux bootstrap` on a new cluster to install flux and connect it to a Git Repository.

### GitOpsSets controller
