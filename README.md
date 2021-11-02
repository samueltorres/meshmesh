# meshmesh

Meshmesh is a tool that allows you to manage [Cilium Clustermesh](https://docs.cilium.io/en/v1.10/gettingstarted/clustermesh/clustermesh/) configurations without hassle.

Meshmesh uses a gossip protocol to manage Cilium Clustermesh members, which makes Clustermesh configuration management a whole lot easier and scalable.

# Introduction

Cilium Clustermesh is a great piece of software, although there are some challenges of configuring Cilium Clustermesh

## GitOps 

The Cilium Cli is really great and you're able to easily install Cilium, enable Clustermesh and establish Clustermesh connections.

But if you're using GitOps to configure your Kubernetes clusters, you'll most likely not have the Cilium Cli at your hands, and all the hard work of setting up the mesh needs to be on Git, which is not easy to accomplish.

This tool makes it easy to make a cluster to join a mesh and get all the needed configuration from the other clusters members. Just installing this tool on the cluster and configure a set of meshmesh seed clusters, i.e another known peer, should be enough.

## mTLS Certificates distribution

For Cilium Clustermesh connections to be correctly established each cluster in the mesh needs to have all the mTLS certificates of all the other clusters.

Distributing the mTLS certificates of all clusters between all clusters is hard to accomplish.

As this tool uses a gossip protocol to exchange membership data, we can safely exchange the Clustermesh mTLS certificates between mesh members, and each cluster only needs to know its own certificate.

## Dynamic cluster members environment

If your mesh members are very dynamic where new clusters come and go regularly, you'll need a way to enable Clustermesh connections between the existing clusters and the new ones. Same goes to when you need to decomission clusters and you need disable the Clustermesh connections.

This tool also makes it easy to set up this environment as a new cluster can join the mesh without the existing clusters in the mesh having to know beforehand their mTLS material and addresses.

# pre-requisites

- [Cilium Clustermesh API Server installed](https://github.com/cilium/cilium/tree/master/clustermesh-apiserver)
- [Shared Certificate Authority installed](https://docs.cilium.io/en/v1.10/gettingstarted/clustermesh/clustermesh/#shared-certificate-authority)
- Cilium Clustermesh API Server Certificates 

In order for meshmesh to do its work, you first need the cluster mesh api server installed and correctly configured.

You should also have a shared certificate authority available in a secret in all of the clusters.

# Installation

todo

# Development

# Todo

- Add more configuration flags
- Add support for dns address advertisement
- Add support for clustermesh api server dns instead of ip + hosts config
- Add testing support
- Add support to remove clusters from the mesh

# Support

This project is still on alpha / idea phase, but it would be nice to gather feedback from the community :)
