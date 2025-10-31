#!/bin/bash

# Kube-SlackGenie-Operator Production Deployment Script
# Usage: ./deploy-production.sh [ENVIRONMENT] [IMAGE_TAG]
# Example: ./deploy-production.sh eks v1.0.0

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
ENVIRONMENT=${1:-"production"}
IMAGE_TAG=${2:-"latest"}
NAMESPACE="kube-slackgenie-operator"

echo -e "${BLUE}🚀 Kube-SlackGenie-Operator Production Deployment${NC}"
echo "Environment: $ENVIRONMENT"
echo "Image Tag: $IMAGE_TAG"
echo "Namespace: $NAMESPACE"
echo ""

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to prompt for input
prompt_input() {
    local prompt="$1"
    local var_name="$2"
    local default_value="$3"
    
    if [ -z "${!var_name}" ]; then
        echo -e "${YELLOW}$prompt${NC}"
        if [ -n "$default_value" ]; then
            read -p "Enter value (default: $default_value): " input
            eval "$var_name=\${input:-$default_value}"
        else
            read -p "Enter value: " input
            eval "$var_name=\$input"
        fi
    fi
}

# Check prerequisites
echo -e "${YELLOW}📋 Checking prerequisites...${NC}"

if ! command_exists kubectl; then
    echo -e "${RED}❌ kubectl is not installed${NC}"
    exit 1
fi

if ! command_exists docker; then
    echo -e "${RED}❌ Docker is not installed${NC}"
    exit 1
fi

if ! kubectl cluster-info &>/dev/null; then
    echo -e "${RED}❌ kubectl is not connected to a cluster${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Prerequisites check passed${NC}"
echo ""

# Get cluster information
CLUSTER_NAME=$(kubectl config current-context)
echo -e "${BLUE}📊 Current cluster: $CLUSTER_NAME${NC}"

# Configure image registry based on environment
case "$ENVIRONMENT" in
    "eks"|"aws")
        echo -e "${YELLOW}🔧 Configuring for AWS EKS...${NC}"
        prompt_input "AWS Account ID:" AWS_ACCOUNT_ID
        prompt_input "AWS Region:" AWS_REGION "us-west-2"
        IMAGE_REPO="${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/kube-slackgenie-operator"
        
        echo "Logging into ECR..."
        aws ecr get-login-password --region $AWS_REGION | docker login --username AWS --password-stdin $AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com
        ;;
    "gke"|"gcp")
        echo -e "${YELLOW}🔧 Configuring for Google GKE...${NC}"
        prompt_input "GCP Project ID:" GCP_PROJECT_ID
        IMAGE_REPO="gcr.io/${GCP_PROJECT_ID}/kube-slackgenie-operator"
        
        echo "Configuring Docker for GCR..."
        gcloud auth configure-docker
        ;;
    "aks"|"azure")
        echo -e "${YELLOW}🔧 Configuring for Azure AKS...${NC}"
        prompt_input "Azure Container Registry:" ACR_NAME
        IMAGE_REPO="${ACR_NAME}.azurecr.io/kube-slackgenie-operator"
        
        echo "Logging into ACR..."
        az acr login --name $ACR_NAME
        ;;
    "dockerhub"|"production")
        echo -e "${YELLOW}🔧 Configuring for Docker Hub...${NC}"
        prompt_input "Docker Hub Username:" DOCKER_USERNAME
        IMAGE_REPO="${DOCKER_USERNAME}/kube-slackgenie-operator"
        
        echo "Please ensure you're logged into Docker Hub:"
        docker login
        ;;
    *)
        echo -e "${YELLOW}🔧 Using custom registry...${NC}"
        prompt_input "Container Registry URL:" IMAGE_REPO
        ;;
esac

FULL_IMAGE="${IMAGE_REPO}:${IMAGE_TAG}"
echo -e "${BLUE}📦 Target image: $FULL_IMAGE${NC}"
echo ""

# Get Slack webhook URL
if [ -z "$SLACK_WEBHOOK_URL" ]; then
    echo -e "${YELLOW}🔗 Slack Configuration${NC}"
    echo "Please provide your Slack incoming webhook URL."
    echo "You can create one at: https://api.slack.com/apps"
    echo ""
    prompt_input "Slack Webhook URL:" SLACK_WEBHOOK_URL
fi

# Validate webhook URL
if [[ ! "$SLACK_WEBHOOK_URL" =~ ^https://hooks\.slack\.com/services/ ]]; then
    echo -e "${RED}❌ Invalid Slack webhook URL format${NC}"
    exit 1
fi

# Build and push image
echo -e "${YELLOW}🔨 Building and pushing Docker image...${NC}"
make docker-build IMG=$FULL_IMAGE
make docker-push IMG=$FULL_IMAGE

# Create namespace
echo -e "${YELLOW}🏗️ Setting up Kubernetes resources...${NC}"
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# Create secret
echo -e "${YELLOW}🔐 Creating Slack webhook secret...${NC}"
kubectl create secret generic slack-webhook \
    --from-literal=webhook-url="$SLACK_WEBHOOK_URL" \
    --namespace=$NAMESPACE \
    --dry-run=client -o yaml | kubectl apply -f -

# Deploy operator
echo -e "${YELLOW}🚀 Deploying operator...${NC}"
make deploy IMG=$FULL_IMAGE

# Wait for deployment
echo -e "${YELLOW}⏳ Waiting for deployment to be ready...${NC}"
kubectl wait --for=condition=available --timeout=300s deployment/controller-manager -n $NAMESPACE || {
    echo -e "${RED}❌ Deployment failed or timed out${NC}"
    echo "Checking pod status..."
    kubectl get pods -n $NAMESPACE
    kubectl describe pod -l control-plane=controller-manager -n $NAMESPACE
    exit 1
}

# Verify deployment
echo -e "${YELLOW}✅ Verifying deployment...${NC}"
READY_PODS=$(kubectl get pods -n $NAMESPACE -l control-plane=controller-manager -o jsonpath='{.items[0].status.containerStatuses[0].ready}')

if [ "$READY_PODS" = "true" ]; then
    echo -e "${GREEN}✅ Operator deployed successfully!${NC}"
else
    echo -e "${RED}❌ Operator pod is not ready${NC}"
    kubectl get pods -n $NAMESPACE
    exit 1
fi

# Show status
echo ""
echo -e "${BLUE}📊 Deployment Status:${NC}"
kubectl get all -n $NAMESPACE

# Show logs
echo ""
echo -e "${BLUE}📋 Recent operator logs:${NC}"
kubectl logs -n $NAMESPACE deployment/controller-manager --tail=10

# Create test pods option
echo ""
echo -e "${YELLOW}🧪 Would you like to deploy test pods to verify alerting? (y/n)${NC}"
read -p "Deploy test pods? " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}🧪 Deploying test pods...${NC}"
    
    kubectl create namespace test-failures --dry-run=client -o yaml | kubectl apply -f -
    
    cat <<EOF | kubectl apply -f -
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

    echo ""
    echo -e "${YELLOW}👀 Watch for test pod failures:${NC}"
    echo "kubectl get pods -n test-failures -w"
    echo ""
    echo -e "${YELLOW}📋 Monitor operator logs for alerts:${NC}"
    echo "kubectl logs -f deployment/controller-manager -n $NAMESPACE"
    echo ""
    echo -e "${YELLOW}🧹 Cleanup test pods when done:${NC}"
    echo "kubectl delete namespace test-failures"
fi

echo ""
echo -e "${GREEN}🎉 Deployment completed successfully!${NC}"
echo ""
echo -e "${BLUE}📝 Next steps:${NC}"
echo "1. Monitor operator logs: kubectl logs -f deployment/controller-manager -n $NAMESPACE"
echo "2. Check Slack channel for alerts"
echo "3. Monitor cluster for pod failures"
echo ""
echo -e "${BLUE}🧹 Cleanup commands (if needed):${NC}"
echo "- Remove operator: make undeploy"
echo "- Remove test pods: kubectl delete namespace test-failures"
echo "- Remove operator namespace: kubectl delete namespace $NAMESPACE"
echo ""
echo -e "${GREEN}✨ Kube-SlackGenie-Operator is now monitoring your cluster!${NC}"
