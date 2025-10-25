# Scout Kubernetes Deployment for Minikube

This directory contains Kubernetes manifests optimized for local development with Minikube.

## 🏗️ Architecture

Scout is a microservices-based system with four components:

| Service | Port | Purpose | Database |
|---------|------|---------|----------|
| **webserver** | 22920 | Central coordinator, REST API, scheduling | SQLite (scheduler.sqlite) |
| **tvdb-proxy** | 22000 | TVDB API proxy, media search | None |
| **torrenter** | 22001 | Download processor, qBittorrent integration | SQLite (db.sqlite) |
| **ui** | 80 | React frontend | None |

### Service Communication
Services communicate via Kubernetes DNS:
- `webserver` → `tvdb-proxy-service:22000`
- `webserver` → `torrenter-service:22001`
- `ui` → `webserver-service:22920` (via Ingress)

## 🚀 Quick Start

### 1. Start Minikube
```bash
minikube start --cpus=4 --memory=8192
```

### 2. Deploy Everything
```bash
./deploy-minikube.sh
```

This script will:
- ✅ Check Minikube is running
- ✅ Enable ingress addon
- ✅ Build all Docker images in Minikube's Docker daemon
- ✅ Apply all Kubernetes manifests
- ✅ Wait for all pods to be ready

### 3. Access the Application

**🎯 NEW: Ingress-Based Access (Recommended)**

For LAN access from any device:
```bash
# Run the complete deployment script
./deploy-complete.sh

# This will set up Ingress and offer to start port forwarding
# Access from any LAN device: http://<YOUR_LAN_IP>:8080
```

See [`INGRESS_SETUP.md`](./INGRESS_SETUP.md) for complete documentation.

**Alternative: Local Development with host-based routing**

Add to `/etc/hosts`:
```bash
echo "$(minikube ip) scout.local" | sudo tee -a /etc/hosts
```

Then access:
- **UI**: http://scout.local
- **API**: http://scout.local/api

## 🔧 Development Workflow

### Quick Commands (using dev.sh)

```bash
# Rebuild a service after code changes
./dev.sh rebuild webserver

# View live logs
./dev.sh logs torrenter

# Check all pod status
./dev.sh status

# Restart a service
./dev.sh restart tvdb-proxy

# Open a shell in a pod
./dev.sh shell webserver

# Port forward for direct access
./dev.sh port-forward

# Clean all resources
./dev.sh clean
```

### Manual Development Commands

```bash
# Set Docker to use Minikube's daemon
eval $(minikube docker-env)

# Rebuild a single service
docker build -t scout-webserver:latest ./webserver
kubectl rollout restart deployment/webserver

# View logs
kubectl logs -f deployment/webserver

# Describe pod issues
kubectl describe pod <pod-name>

# Check all resources
kubectl get all
```

## 📦 Configuration Strategy

### Why ConfigMaps + Secrets (Not TOML in Images)?

**Current Approach:**
- ✅ **ConfigMaps**: Non-sensitive settings (URLs, ports, section numbers)
- ✅ **Secrets**: Sensitive data (API keys, passwords, tokens) - base64 encoded
- ✅ **TOML Files**: Mounted from ConfigMaps at runtime

**Benefits:**
1. **No Image Rebuilds**: Update configs without rebuilding Docker images
2. **Environment-Specific**: Different configs for dev/staging/prod
3. **Security**: Secrets separate from code and configs
4. **K8s Native**: Follows Kubernetes best practices
5. **Version Control**: Can track config changes in Git (except secrets)

**For Local Development:**
- Original TOML files remain in service directories
- K8s ConfigMaps mirror the TOML structure
- Services read from mounted ConfigMaps in containers
- Secrets injected as separate files in `/root/secrets`

### Configuration Loading

Each service loads config in this order:
1. Read base config from TOML (mounted from ConfigMap)
2. Override sensitive values from Secret files
3. Override service URLs from environment variables

Example (tvdb_proxy):
```go
// Load base config from api.toml (ConfigMap)
conf, err := toml.LoadFile("api.toml")

// Override with secrets
if apiKey, err := os.ReadFile("secrets/api-key"); err == nil {
    tvDbConfig.ApiKey = string(apiKey)
}
```

## 🔐 Security Features

### Secrets Management

Sensitive data is stored in Kubernetes Secrets (base64 encoded):

**tvdb-proxy-secrets:**
- `api-key`: TVDB API key
- `token`: JWT authentication token

**torrenter-secrets:**
- `prowlarr-key`: Prowlarr API key
- `qbitt-user`: qBittorrent username
- `qbitt-password`: qBittorrent password
- `plex-key`: Plex authentication token

### Updating Secrets

```bash
# Encode new value
echo -n "new-secret-value" | base64

# Edit secret YAML file
vim k8s/secrets/tvdb-proxy-secrets.yaml

# Apply and restart
kubectl apply -f k8s/secrets/
kubectl rollout restart deployment/tvdb-proxy
```

## 💾 Persistent Storage

SQLite databases are stored on PersistentVolumeClaims:

- `webserver-db-pvc`: Mounted at `/root/data/scheduler.sqlite`
- `torrenter-db-pvc`: Mounted at `/root/data/db.sqlite`

**Data Persistence:**
- Data survives pod restarts
- Minikube stores PVs in `/tmp/hostpath-provisioner/`
- Data is lost if Minikube is deleted

**Backup Database:**
```bash
kubectl cp webserver-<pod-name>:/root/data/scheduler.sqlite ./backup.sqlite
```

## 🐳 Image Management for Minikube

### Why `imagePullPolicy: Never`?

For local development, images are built directly in Minikube's Docker daemon:
```bash
eval $(minikube docker-env)
docker build -t scout-webserver:latest ./webserver
```

With `imagePullPolicy: Never`, Kubernetes:
- ✅ Uses local images only
- ✅ Never tries to pull from remote registry
- ✅ Fails fast if image doesn't exist locally

**For Production**: Change to `imagePullPolicy: Always` and use a registry.

## 🌐 Networking

### Service Discovery
Services use Kubernetes DNS for inter-service communication:
- Format: `<service-name>.<namespace>.svc.cluster.local`
- Short form: `<service-name>` (same namespace)

### Ingress Configuration
```yaml
Host: scout.local
Paths:
  /     → ui-service:80
  /api  → webserver-service:22920
```

### Port Forwarding (Alternative Access)
```bash
# Access services without ingress
kubectl port-forward deployment/webserver 22920:22920
kubectl port-forward deployment/ui 8080:80
```

## 🐛 Troubleshooting

### Pod Not Starting
```bash
# Check pod status
kubectl get pods

# Describe pod for events
kubectl describe pod <pod-name>

# Check logs
kubectl logs <pod-name>

# Check if image exists in Minikube
minikube ssh "docker images | grep scout"
```

### Service Not Reachable
```bash
# Test service internally
kubectl run -it --rm debug --image=alpine --restart=Never -- sh
> apk add curl
> curl http://tvdb-proxy-service:22000/

# Check service endpoints
kubectl get endpoints
```

### Database Issues
```bash
# Check PVC status
kubectl get pvc

# Check PV
kubectl get pv

# Access database
kubectl exec -it deployment/webserver -- ls -la /root/data/
```

### ConfigMap/Secret Not Loading
```bash
# Verify ConfigMap exists
kubectl get configmap tvdb-proxy-config -o yaml

# Verify Secret exists
kubectl get secret tvdb-proxy-secrets -o yaml

# Check if mounted in pod
kubectl exec -it deployment/tvdb-proxy -- ls -la /root/
kubectl exec -it deployment/tvdb-proxy -- cat /root/api.toml
```

## 📊 Monitoring

```bash
# Resource usage
kubectl top pods

# Watch pod status
kubectl get pods -w

# All events
kubectl get events --sort-by='.lastTimestamp'
```

## 🧹 Cleanup

```bash
# Delete all Scout resources
./dev.sh clean

# Or manually
kubectl delete -f k8s/

# Stop Minikube
minikube stop

# Delete Minikube (WARNING: deletes all data)
minikube delete
```

## 🔗 External Dependencies

Update these in `k8s/configmaps/torrenter-configmap.yaml`:

- **qBittorrent**: `http://qbittorrent-service:8080`
- **Plex**: `http://plex-service:32400`
- **Prowlarr**: `http://prowlarr-service:9696`

For local development, these may point to host services:
```yaml
# Use host.minikube.internal to reach host machine
host: "http://host.minikube.internal:8080"
```

## 📚 Additional Resources

- [Minikube Documentation](https://minikube.sigs.k8s.io/docs/)
- [Kubernetes ConfigMaps](https://kubernetes.io/docs/concepts/configuration/configmap/)
- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
- [Scout Architecture](../README.md)