---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "consul_peering Resource - terraform-provider-consul"
subcategory: ""
description: |-
  Cluster Peering https://www.consul.io/docs/connect/cluster-peering can be used to create connections between two or more independent clusters so that services deployed to different partitions or datacenters can communicate.
  The cluster_peering resource can be used to establish the peering after a peering token has been generated.
  ~> Cluster peering is currently in technical preview: Functionality associated with cluster peering is subject to change. You should never use the technical preview release in secure environments or production scenarios. Features in technical preview may have performance issues, scaling issues, and limited support.
  The functionality described here is available only in Consul version 1.13.0 and later.
---

# consul_peering (Resource)

[Cluster Peering](https://www.consul.io/docs/connect/cluster-peering) can be used to create connections between two or more independent clusters so that services deployed to different partitions or datacenters can communicate.

The `cluster_peering` resource can be used to establish the peering after a peering token has been generated.

~> **Cluster peering is currently in technical preview:** Functionality associated with cluster peering is subject to change. You should never use the technical preview release in secure environments or production scenarios. Features in technical preview may have performance issues, scaling issues, and limited support.

The functionality described here is available only in Consul version 1.13.0 and later.

## Example Usage

```terraform
# Create a peering between the EU and US Consul clusters

provider "consul" {
  alias   = "eu"
  address = "eu-cluster:8500"
}

provider "consul" {
  alias   = "us"
  address = "us-cluster:8500"
}

resource "consul_peering_token" "eu-us" {
  provider  = consul.us
  peer_name = "eu-cluster"
}

resource "consul_peering" "eu-us" {
  provider = consul.eu

  peer_name     = "eu-cluster"
  peering_token = consul_peering_token.token.peering_token

  meta = {
    hello = "world"
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `peer_name` (String) The name assigned to the peer cluster. The `peer_name` is used to reference the peer cluster in service discovery queries and configuration entries such as `service-intentions`. This field must be a valid DNS hostname label.
- `peering_token` (String, Sensitive) The peering token fetched from the peer cluster.

### Optional

- `meta` (Map of String) Specifies KV metadata to associate with the peering. This parameter is not required and does not directly impact the cluster peering process.
- `partition` (String)

### Read-Only

- `deleted_at` (String)
- `exported_service_count` (Number)
- `id` (String) The ID of this resource.
- `imported_service_count` (Number)
- `peer_ca_pems` (List of String)
- `peer_id` (String)
- `peer_server_addresses` (List of String)
- `peer_server_name` (String)
- `state` (String)


