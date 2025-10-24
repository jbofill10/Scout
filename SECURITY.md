# Security Configuration Guide

This document explains how to properly configure Scout for deployment without exposing sensitive credentials.

## ⚠️ Important Security Notes

- **Never commit actual secrets or configs to version control**
- All files in `k8s/secrets/*.yaml` and `k8s/configmaps/*.yaml` (except `.template.yaml` files) are gitignored
- Rotate all credentials if you suspect they've been exposed
- Use strong, unique passwords for all services

## Required Configuration

Scout requires both secrets (sensitive data) and configmaps (environment-specific settings) to be configured.

## ConfigMaps

### 1. Torrenter ConfigMap
**File:** `k8s/configmaps/torrenter-configmap.yaml`

Create from template:
```bash
cp k8s/configmaps/torrenter-configmap.template.yaml k8s/configmaps/torrenter-configmap.yaml
```

Update the following values:
- `prowlarr-host`: Replace `<YOUR_LAN_IP>` with your LAN IP (e.g., `http://192.168.0.111:9696`)
- `plex-host`: Replace `<YOUR_LAN_IP>` with your LAN IP (e.g., `http://192.168.0.111:32400`)
- `plex-movie-sections`: Your Plex movie library section ID (check Plex settings)
- `plex-show-sections`: Your Plex TV show library section ID (check Plex settings)

### 2. TVDB Proxy ConfigMap
**File:** `k8s/configmaps/tvdb-proxy-configmap.yaml`

Create from template:
```bash
cp k8s/configmaps/tvdb-proxy-configmap.template.yaml k8s/configmaps/tvdb-proxy-configmap.yaml
```

This file typically doesn't need changes unless TVDB changes their API endpoint.

## Secrets

Scout requires the following secrets to operate:

### 1. PostgreSQL Database Credentials
**File:** `k8s/secrets/postgres-secret.yaml`

Create from template:
```bash
cp k8s/secrets/postgres-secret.template.yaml k8s/secrets/postgres-secret.yaml
```

Required values:
- `POSTGRES_DB`: Database name (e.g., `scoutdb`)
- `POSTGRES_USER`: Database username (e.g., `scoutuser`)
- `POSTGRES_PASSWORD`: Strong database password

### 2. Torrenter Service Secrets
**File:** `k8s/secrets/torrenter-secrets.yaml`

Create from template:
```bash
cp k8s/secrets/torrenter-secrets.template.yaml k8s/secrets/torrenter-secrets.yaml
```

Required values:
- `prowlarr-key`: Your Prowlarr API key (Settings → General → API Key)
- `qbitt-user`: qBittorrent WebUI username
- `qbitt-password`: qBittorrent WebUI password
- `plex-key`: Your Plex authentication token

#### Getting Your Plex Token
1. Visit: https://www.plex.tv/claim/
2. Sign in and copy the claim token
3. Or extract from existing Plex installation:
   - Check `~/.config/plex/Preferences.xml` for `PlexOnlineToken` attribute
   - Or use: https://support.plex.tv/articles/204059436-finding-an-authentication-token-x-plex-token/

### 3. TVDB Proxy Secrets
**File:** `k8s/secrets/tvdb-proxy-secrets.yaml`

Create from template:
```bash
cp k8s/secrets/tvdb-proxy-secrets.template.yaml k8s/secrets/tvdb-proxy-secrets.yaml
```

Required values:
- `api-key`: TVDB API v4 key
- `token`: TVDB JWT bearer token

#### Getting TVDB Credentials
1. Create account at https://thetvdb.com/
2. Subscribe to API access: https://thetvdb.com/api-information
3. Generate API key from your dashboard
4. Use the API key to obtain a JWT token:

```bash
curl -X POST https://api4.thetvdb.com/v4/login \
  -H "Content-Type: application/json" \
  -d '{"apikey": "YOUR_API_KEY"}'
```

The response will contain a `token` field - this is your JWT bearer token.

## Base64 Encoding Secrets

Kubernetes secrets require base64-encoded values. Encode your secrets:

```bash
# Encode a value
echo -n "your_secret_value" | base64

# Decode to verify
echo "encoded_value" | base64 -d
```

**Important:** Use `echo -n` to avoid including a newline character.

## Environment-Specific Configuration

While secrets contain sensitive credentials, ConfigMaps contain environment-specific settings like IP addresses and port numbers. Both are gitignored to keep the repository environment-agnostic.

### ConfigMaps
Update `k8s/configmaps/torrenter-configmap.yaml` (created from template):
- Replace `<YOUR_LAN_IP>` with your actual LAN IP address
- Update Plex section IDs to match your library sections

ConfigMaps don't require base64 encoding - values are stored as plain text.

### Environment Variables
The webserver accepts:
- `BIND_ADDRESS`: Address to bind to (default: `0.0.0.0:22920`)
- `TVDB_HOST`: TVDB proxy endpoint
- `TORRENTER_HOST`: Torrenter service endpoint
- `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`: Database connection

## Deployment Checklist

Before deploying Scout:

- [ ] Copy all ConfigMap `.template.yaml` files and fill in your environment values
- [ ] Copy all Secret `.template.yaml` files and fill in actual credentials
- [ ] Base64 encode all secret values
- [ ] Update ConfigMaps with your LAN IP address and Plex section IDs
- [ ] Verify `.gitignore` excludes actual config and secret files
- [ ] Never commit files containing real credentials or environment-specific configs
- [ ] Test deployment in a non-production environment first

## Production Best Practices

### Secret Management
For production deployments, consider:

1. **Sealed Secrets**: Encrypt secrets for safe Git storage
   ```bash
   kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.18.0/controller.yaml
   ```

2. **External Secrets Operator**: Sync from external secret stores
   - AWS Secrets Manager
   - HashiCorp Vault
   - Azure Key Vault
   - Google Secret Manager

3. **K8s Secret Management Tools**:
   - SOPS (Secrets OPerationS)
   - Helm Secrets
   - Kustomize with secret generators

### Network Security
- Use Network Policies to restrict pod-to-pod communication
- Enable TLS/SSL for all external endpoints
- Consider a service mesh (Istio, Linkerd) for mTLS
- Don't expose services directly - use ingress with authentication

### Access Control
- Use RBAC to limit service account permissions
- Implement pod security policies/standards
- Regular credential rotation (automated if possible)
- Monitor access logs for suspicious activity

## Credential Rotation

If credentials are compromised:

1. **Immediate Actions:**
   - Revoke/regenerate all API keys
   - Change all passwords
   - Review access logs for unauthorized activity

2. **Update Secrets:**
   ```bash
   # Delete old secret
   kubectl delete secret <secret-name>

   # Create new secret with updated values
   kubectl apply -f k8s/secrets/<secret-name>.yaml

   # Restart affected pods
   kubectl rollout restart deployment/<deployment-name>
   ```

3. **Verify:**
   - Check pod logs for successful authentication
   - Test all service integrations
   - Monitor for any errors

## Support

For security concerns or questions:
- Open an issue (without including sensitive data)
- Review the main README.md for general setup
- Check service logs for configuration errors

## Reporting Security Vulnerabilities

If you discover a security vulnerability, please:
1. **Do NOT** open a public issue
2. Contact the maintainer directly via GitHub
3. Provide detailed information about the vulnerability
4. Allow time for a fix before public disclosure
