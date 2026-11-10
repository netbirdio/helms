# Netbird Self-Hosted Setup

This example provides a fully configured and tested setup for deploying Netbird using the following components:

- **Ingress Controller**: Nginx
- **Database Storage**: PostgreSQL
- **Identity Provider**: Azure Active Directory (Azure AD)

## Prerequisites

Before starting the setup, refer to the [Netbird documentation](https://docs.netbird.io/selfhosted/identity-providers#azure-ad-microsoft-entra-id) to configure your Azure Active Directory Identity Provider and generate the necessary parameters:

- `idpClientID`
- `idpClientSecret`
- `idpDirectoryId` (Tenant ID)
- `idpObjectId`
- `idpAuthAuthority`
- `idpOidcScope`
- PostgreSQL database connection string

Additionally, you will need to configure the Azure AD application registration as part of the documented steps for setting up the Azure AD IDP.

## Kubernetes Secret Configuration

This setup requires Kubernetes secrets to store sensitive data. You'll need to create a secret named `netbird` in your Kubernetes cluster, containing the following key-value pairs:

- `idpClientID`: `xxxxxx` # The `clientId` from the Azure AD application registration.
- `idpClientSecret`: `xxxxxx` # The `clientSecret` from the Azure AD application registration.
- `idpDirectoryId`: `xxxxxx` # The Azure AD Tenant ID (Directory ID).
- `idpObjectId`: `xxxxxx` # The Azure AD Object ID of the application.
- `idpAuthAuthority`: `xxxxxx` # The Azure AD authority URL, e.g., `https://login.microsoftonline.com/{idpDirectoryId}/v2.0`.
- `idpOidcScope`: `xxxxxx` # The OIDC scopes for Azure AD, e.g., `openid profile email offline_access User.Read api://{idpClientID}/api`.
- `postgresDSN`: `xxxxxx` # PostgreSQL database connection string, e.g., `postgres://user:password@host:port/database?sslmode=require`.
- `relayPassword`: `xxxxxx` # Password used to secure communication between peers in the relay service.
- `stunServer`: `xxxxxx` # STUN server URL, e.g., `stun:stun.myexample.com:3478`.
- `turnServer`: `xxxxxx` # TURN server URL, e.g., `turn:turn.myexample.com:3478`.
- `turnServerUser`: `xxxxxx` # TURN server username.
- `turnServerPassword`: `xxxxxx` # TURN server password.
- `datastoreEncryptionKey`: `xxxxxxx` # A random encryption key for the datastore, e.g., generated via `openssl rand -base64 32`.

You will also need to ensure that your PostgreSQL database is accessible from your Kubernetes cluster and properly configured with the connection string in the `postgresDSN` secret.

> **Note:** The `datastoreEncryptionKey` must also be provided in a ConfigMap for the Netbird setup.
>
> **Important:** This configuration uses PostgreSQL as the database backend instead of SQLite. Ensure your PostgreSQL database is properly set up and accessible before deployment.

## Deployment

Once the required secrets and configuration are in place, this setup will deploy all necessary services for running Netbird, including the following exposed endpoints:

- `netbird-dashboard.example.com` - The Netbird dashboard.
- `netbird.example.com` - The main Netbird services (management|relay|signal).

## Additional info

Starting with Netbird v0.30.1, the platform supports reading environment variables directly within the `management.json` file. In this example, we leverage this feature by defining environment variables in the following format: `{{ .EnvVarName }}`.
