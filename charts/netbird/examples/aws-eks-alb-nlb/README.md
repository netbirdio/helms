# Netbird Self-Hosted Setup

This example provides a fully configured and tested setup for deploying Netbird using the following components:

- **Ingress Controller**: AWS ALB (HTTP) and NLB (STUN)
- **Database Storage**: PostgreSQL
- **Identity Provider**: Embedded (Dex)

## Prerequisites

This setup assumes you have an existing AWS EKS cluster (with the AWS Load Balancer Controller installed) and a PostgreSQL database installed and configured.

## Kubernetes Secret Configuration

This setup requires Kubernetes secrets to store sensitive data. You'll need to create a secret named `netbird` in your Kubernetes cluster, containing the following key-value pairs:

- `relayAuthSecret`: `xxxxxx` # Password used to secure communication between peers in the relay service.
- `datastoreDsnPassword`: `xxxxxx` # Password for the PostgreSQL database connection.
- `datastoreEncryptionKey`: `xxxxxxx` # A random encryption key for the datastore, e.g., generated via `openssl rand -base64 32`.

> **Note:** The `datastoreEncryptionKey` must also be provided in a ConfigMap for the Netbird setup.

## Deployment

Once the required secrets and configuration are in place, this setup will deploy all necessary services for running Netbird, including the following exposed endpoints:

- `netbird.example.com` - The main Netbird services (dashboard|server).

## Additional info

While this setup also deploys the embedded STUN server, you will likely need to use a separate hostname for the ELB (since STUN cannot be served by ALB). It does not seem like NetBird allows configuring a separate hostname for the STUN server; it may be easier to simply use a public STUN server and configure it under `stuns` in the `config.yaml`.
