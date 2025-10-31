# Production Deployment Guide - Kube-SlackGenie-Operator

This guide provides step-by-step instructions to deploy the Kube-SlackGenie-Operator in production Kubernetes environments (EKS, GKE, AKS, or any managed/self-hosted cluster).

## Prerequisites 📋

- Kubernetes cluster (v1.19+) with admin access
- `kubectl` configured and connected to your cluster
- Docker or container registry access
- Slack workspace with webhook creation permissions
- `make` utility installed

## Quick Production Deployment 🚀

### Step 1: Prepare Slack Integration

1. **Create Slack App**:
   ```bash
   # Visit: https://api.slack.com/apps
   # Click "Create New App" → "From scratch"
   ```

2. **Configure Incoming Webhook**:
   ```bash
   # In your Slack App:
   # 1. Go to "Incoming Webhooks"
   # 2. Toggle "Activate Incoming Webhooks" to ON
   # 3. Click "Add New Webhook to Workspace"
   # 4. Select target channel (e.g., #alerts, #devops, #production)
   # 5. Copy the webhook URL
   ```

3. **Save Webhook URL**:
   ```bash
   export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
   echo $SLACK_WEBHOOK_URL  # Verify it's set
   ```

### Step 2: Clone and Prepare Repository

```bash
# Clone the repository
git clone https://github.com/ahmadrazalab/kube-slackgenie-operator.git
cd kube-slackgenie-operator

# Verify kubectl connection
kubectl cluster-info
kubectl get nodes
```

### Step 3: Build and Push Docker Image

**Option A: Using Docker Hub**
```bash
# Set your Docker Hub username
export DOCKER_USERNAME="your-dockerhub-username"

# Build and push image
make docker-build IMG=${DOCKER_USERNAME}/kube-slackgenie-operator:v1.0.0
make docker-push IMG=${DOCKER_USERNAME}/kube-slackgenie-operator:v1.0.0
```

**Option B: Using Private Registry (AWS ECR)**
```bash
# Configure AWS CLI and login to ECR
aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-west-2.amazonaws.com

# Set your ECR repository
export ECR_REPO="123456789012.dkr.ecr.us-west-2.amazonaws.com/kube-slackgenie-operator"

# Build and push
make docker-build IMG=${ECR_REPO}:v1.0.0
make docker-push IMG=${ECR_REPO}:v1.0.0
```

**Option C: Using Private Registry (Google GCR)**
```bash
# Authenticate with GCR
gcloud auth configure-docker

# Set your GCR repository
export GCR_REPO="gcr.io/your-project-id/kube-slackgenie-operator"

# Build and push
make docker-build IMG=${GCR_REPO}:v1.0.0
make docker-push IMG=${GCR_REPO}:v1.0.0
```

**Option D: Using Private Registry (Azure ACR)**
```bash
# Login to ACR
az acr login --name your-registry-name

# Set your ACR repository
export ACR_REPO="your-registry-name.azurecr.io/kube-slackgenie-operator"

# Build and push
make docker-build IMG=${ACR_REPO}:v1.0.0
make docker-push IMG=${ACR_REPO}:v1.0.0
```

### Step 4: Create Production Namespace and Secret

```bash
# Create dedicated namespace
kubectl create namespace kube-slackgenie-operator

# Create Slack webhook secret
kubectl create secret generic slack-webhook \
    --from-literal=webhook-url="$SLACK_WEBHOOK_URL" \
    -n kube-slackgenie-operator

# Verify secret creation
kubectl get secret slack-webhook -n kube-slackgenie-operator -o yaml
```

### Step 5: Configure Production Settings

**Create production kustomization overlay:**
```bash
# Create production overlay directory
mkdir -p config/overlays/production

# Create production kustomization
cat > config/overlays/production/kustomization.yaml << EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- ../../default

namespace: kube-slackgenie-operator

images:
- name: controller
  newName: ${DOCKER_USERNAME}/kube-slackgenie-operator  # Replace with your image
  newTag: v1.0.0

patchesStrategicMerge:
- manager-config.yaml

commonLabels:
  app.kubernetes.io/version: v1.0.0
  app.kubernetes.io/environment: production
EOF
```

**Create production manager configuration:**
```bash
cat > config/overlays/production/manager-config.yaml << EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
  namespace: system
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: SLACK_WEBHOOK_URL
          valueFrom:
            secretKeyRef:
              name: slack-webhook
              key: webhook-url
        resources:
          limits:
            cpu: 200m
            memory: 256Mi
          requests:
            cpu: 50m
            memory: 128Mi
        # Add production-specific configurations
        securityContext:
          runAsNonRoot: true
          runAsUser: 1001
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - "ALL"
EOF
```

### Step 6: Deploy to Production

**Deploy using production overlay:**
```bash
# Deploy the operator
kubectl apply -k config/overlays/production

# Alternative: Use the IMG parameter directly
make deploy IMG=${DOCKER_USERNAME}/kube-slackgenie-operator:v1.0.0
```

### Step 7: Verify Deployment

```bash
# Check namespace resources
kubectl get all -n kube-slackgenie-operator

# Check operator pod status
kubectl get pods -n kube-slackgenie-operator
kubectl describe pod -l control-plane=controller-manager -n kube-slackgenie-operator

# Check operator logs
kubectl logs -f deployment/controller-manager -n kube-slackgenie-operator

# Verify RBAC permissions
kubectl auth can-i list pods --as=system:serviceaccount:kube-slackgenie-operator:controller-manager
kubectl auth can-i watch pods --as=system:serviceaccount:kube-slackgenie-operator:controller-manager
```

### Step 8: Test with Sample Failures

**Create test namespace:**
```bash
kubectl create namespace test-failures
```

**Deploy test pods:**
```bash
cat > test-production.yaml << EOF
apiVersion: v1
kind: Pod
metadata:
  name: test-crashloop
  namespace: test-failures
  labels:
    app: test-pod
spec:
  containers:
  - name: crashloop-container
    image: busybox:latest
    command: ["/bin/sh", "-c", "echo 'Testing crashloop...'; sleep 3; exit 1"]
    resources:
      limits:
        cpu: "100m"
        memory: "64Mi"
      requests:
        cpu: "50m"
        memory: "32Mi"
  restartPolicy: Always
---
apiVersion: v1
kind: Pod
metadata:
  name: test-imagepull
  namespace: test-failures
  labels:
    app: test-pod
spec:
  containers:
  - name: imagepull-container
    image: nonexistent/fake-image:v999
    resources:
      limits:
        cpu: "100m"
        memory: "64Mi"
      requests:
        cpu: "50m"
        memory: "32Mi"
  restartPolicy: Always
EOF

# Deploy test pods
kubectl apply -f test-production.yaml

# Watch for failures
kubectl get pods -n test-failures -w
```

**Monitor operator logs for alerts:**
```bash
kubectl logs -f deployment/controller-manager -n kube-slackgenie-operator | grep -i "slack alert sent"
```

### Step 9: Production Monitoring and Observability

**Setup monitoring (optional):**
```bash
# Check operator metrics endpoint
kubectl port-forward -n kube-slackgenie-operator deployment/controller-manager 8443:8443

# In another terminal, check metrics
curl -k https://localhost:8443/metrics
```

**Setup log aggregation:**
```bash
# If using Fluentd/Fluent Bit, add this label selector:
# kubernetes.labels.control-plane: controller-manager
# kubernetes.namespace_name: kube-slackgenie-operator
```

### Step 10: Production Configuration Tuning

**Adjust debounce window (optional):**
```bash
# Edit the controller code or add environment variable
kubectl set env deployment/controller-manager -n kube-slackgenie-operator DEBOUNCE_WINDOW=15m
```

**Configure resource limits for production:**
```bash
kubectl patch deployment controller-manager -n kube-slackgenie-operator -p '
{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "manager",
            "resources": {
              "limits": {
                "cpu": "500m",
                "memory": "512Mi"
              },
              "requests": {
                "cpu": "100m",
                "memory": "256Mi"
              }
            }
          }
        ]
      }
    }
  }
}'
```

## Environment-Specific Deployment Commands 🌍

### AWS EKS Deployment
```bash
# Configure kubectl for EKS
aws eks update-kubeconfig --region us-west-2 --name your-cluster-name

# Use ECR for image registry
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export AWS_REGION="us-west-2"
export ECR_REPO="${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/kube-slackgenie-operator"

# Build and deploy
make docker-build IMG=${ECR_REPO}:v1.0.0
make docker-push IMG=${ECR_REPO}:v1.0.0
make deploy IMG=${ECR_REPO}:v1.0.0
```

### Google GKE Deployment
```bash
# Configure kubectl for GKE
gcloud container clusters get-credentials your-cluster-name --zone us-central1-a --project your-project-id

# Use GCR for image registry
export GCR_REPO="gcr.io/your-project-id/kube-slackgenie-operator"

# Build and deploy
make docker-build IMG=${GCR_REPO}:v1.0.0
make docker-push IMG=${GCR_REPO}:v1.0.0
make deploy IMG=${GCR_REPO}:v1.0.0
```

### Azure AKS Deployment
```bash
# Configure kubectl for AKS
az aks get-credentials --resource-group your-rg --name your-cluster-name

# Use ACR for image registry
export ACR_REPO="your-registry.azurecr.io/kube-slackgenie-operator"

# Build and deploy
make docker-build IMG=${ACR_REPO}:v1.0.0
make docker-push IMG=${ACR_REPO}:v1.0.0
make deploy IMG=${ACR_REPO}:v1.0.0
```

### Self-Hosted Kubernetes
```bash
# Verify cluster access
kubectl cluster-info

# Use Docker Hub or private registry
export DOCKER_REPO="your-registry.com/kube-slackgenie-operator"

# Build and deploy
make docker-build IMG=${DOCKER_REPO}:v1.0.0
make docker-push IMG=${DOCKER_REPO}:v1.0.0
make deploy IMG=${DOCKER_REPO}:v1.0.0
```

## Production Cleanup Commands 🧹

```bash
# Remove test resources
kubectl delete -f test-production.yaml
kubectl delete namespace test-failures

# Remove operator (if needed)
make undeploy
# OR
kubectl delete -k config/overlays/production

# Remove secrets
kubectl delete secret slack-webhook -n kube-slackgenie-operator

# Remove namespace
kubectl delete namespace kube-slackgenie-operator
```

## Troubleshooting 🔧

### Common Issues and Solutions

**1. Pod ImagePullBackOff**
```bash
# Check if image exists in registry
docker pull your-registry/kube-slackgenie-operator:v1.0.0

# Check image pull secrets (if using private registry)
kubectl get secrets -n kube-slackgenie-operator
kubectl describe pod -l control-plane=controller-manager -n kube-slackgenie-operator
```

**2. RBAC Permission Issues**
```bash
# Verify cluster role and bindings
kubectl get clusterrole | grep slackgenie
kubectl get clusterrolebinding | grep slackgenie
kubectl describe clusterrole controller-manager-role
```

**3. Slack Webhook Issues**
```bash
# Test webhook manually
curl -X POST -H 'Content-type: application/json' \
    --data '{"text":"Test message from Kube-SlackGenie-Operator"}' \
    $SLACK_WEBHOOK_URL

# Check secret format
kubectl get secret slack-webhook -n kube-slackgenie-operator -o yaml
echo "aGV5..." | base64 -d  # Decode the webhook URL
```

**4. Operator Not Starting**
```bash
# Check events
kubectl get events -n kube-slackgenie-operator --sort-by='.lastTimestamp'

# Check logs
kubectl logs -l control-plane=controller-manager -n kube-slackgenie-operator --previous

# Check resource limits
kubectl describe deployment controller-manager -n kube-slackgenie-operator
```

### Health Check Commands

```bash
# Quick health check
kubectl get pods -n kube-slackgenie-operator
kubectl logs -n kube-slackgenie-operator deployment/controller-manager --tail=10

# Detailed status check
kubectl get all -n kube-slackgenie-operator
kubectl describe deployment controller-manager -n kube-slackgenie-operator

# Check if operator is watching pods
kubectl logs -n kube-slackgenie-operator deployment/controller-manager | grep -i "Starting EventSource"
```

## Security Considerations 🔐

### Production Security Best Practices

1. **Use Private Container Registry**
2. **Enable Pod Security Standards**
3. **Implement Network Policies**
4. **Regular Security Updates**
5. **Monitor RBAC Permissions**

```bash
# Apply network policy (example)
cat > network-policy.yaml << EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kube-slackgenie-operator-netpol
  namespace: kube-slackgenie-operator
spec:
  podSelector:
    matchLabels:
      control-plane: controller-manager
  policyTypes:
  - Egress
  egress:
  - {}  # Allow all egress (for Slack webhooks and Kubernetes API)
EOF

kubectl apply -f network-policy.yaml
```

## Maintenance and Updates 🔄

### Updating the Operator

```bash
# Build new version
make docker-build IMG=your-registry/kube-slackgenie-operator:v1.1.0
make docker-push IMG=your-registry/kube-slackgenie-operator:v1.1.0

# Update deployment
kubectl set image deployment/controller-manager -n kube-slackgenie-operator \
    manager=your-registry/kube-slackgenie-operator:v1.1.0

# Verify rollout
kubectl rollout status deployment/controller-manager -n kube-slackgenie-operator
```

### Backup and Restore

```bash
# Backup configuration
kubectl get all -n kube-slackgenie-operator -o yaml > kube-slackgenie-backup.yaml

# Backup secrets (handle carefully)
kubectl get secret -n kube-slackgenie-operator -o yaml > secrets-backup.yaml
```

---

## 🎉 Success Indicators

After successful deployment, you should see:

- ✅ Operator pod running: `kubectl get pods -n kube-slackgenie-operator`
- ✅ No error logs: `kubectl logs -f deployment/controller-manager -n kube-slackgenie-operator`
- ✅ Slack alerts for test pods
- ✅ Debouncing working (duplicate alerts prevented)
- ✅ Proper RBAC permissions

**Your Kube-SlackGenie-Operator is now protecting your production cluster!** 🚀

For support and issues: [GitHub Issues](https://github.com/ahmadrazalab/kube-slackgenie-operator/issues)
