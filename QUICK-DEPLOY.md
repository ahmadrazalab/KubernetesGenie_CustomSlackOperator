# Quick Deployment Reference 🚀

## One-Command Deployments

### AWS EKS
```bash
./deploy-production.sh eks v1.0.0
```

### Google GKE  
```bash
./deploy-production.sh gke v1.0.0
```

### Azure AKS
```bash
./deploy-production.sh aks v1.0.0
```

### Docker Hub / Any Cluster
```bash
./deploy-production.sh production v1.0.0
```

## Manual Deployment (3 Steps)

### Step 1: Set Environment
```bash
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
export IMAGE_REPO="your-registry/kube-slackgenie-operator:v1.0.0"
```

### Step 2: Build & Push
```bash
make docker-build IMG=$IMAGE_REPO
make docker-push IMG=$IMAGE_REPO
```

### Step 3: Deploy
```bash
kubectl create namespace kube-slackgenie-operator
kubectl create secret generic slack-webhook --from-literal=webhook-url="$SLACK_WEBHOOK_URL" -n kube-slackgenie-operator
make deploy IMG=$IMAGE_REPO
```

## Verification Commands

```bash
# Check status
kubectl get pods -n kube-slackgenie-operator

# Check logs
kubectl logs -f deployment/controller-manager -n kube-slackgenie-operator

# Test with failing pod
kubectl run test-fail --image=nonexistent/image:latest --restart=Never
```

## Cleanup Commands

```bash
# Remove operator
make undeploy

# Remove test pods
kubectl delete pod test-fail

# Complete cleanup
kubectl delete namespace kube-slackgenie-operator
```
