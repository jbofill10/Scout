#!/bin/bash

# Enable ingress addon in minikube
echo "Enabling ingress addon in minikube..."
minikube addons enable ingress

# Wait for ingress controller to be ready
echo "Waiting for ingress controller to be ready..."
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=120s

# Get the minikube IP
MINIKUBE_IP=$(minikube ip)
echo ""
echo "Minikube IP: $MINIKUBE_IP"

# Get the NodePort for the ingress controller
INGRESS_PORT=$(kubectl get svc ingress-nginx-controller -n ingress-nginx -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}')
echo "Ingress HTTP NodePort: $INGRESS_PORT"

echo ""
echo "========================================"
echo "Ingress setup complete!"
echo "========================================"
echo ""
echo "Access your application at: http://$MINIKUBE_IP:$INGRESS_PORT"
echo ""
echo "To expose on your LAN IP (<YOUR_LAN_IP>:8080), you can use:"
echo "  socat TCP-LISTEN:8080,fork,reuseaddr,bind=<YOUR_LAN_IP> TCP:$MINIKUBE_IP:$INGRESS_PORT"
echo ""
echo "Or set up a more permanent port forward:"
echo "  kubectl port-forward -n ingress-nginx service/ingress-nginx-controller --address=0.0.0.0 8080:80"
echo ""
