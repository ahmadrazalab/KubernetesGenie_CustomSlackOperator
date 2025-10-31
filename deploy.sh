#!/bin/bash

# Kube-SlackGenie-Operator Deployment Script
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="kube-slackgenie-operator-system"
IMG="${IMG:-ahmadrazalab/kube-slackgenie-operator:latest}"

echo -e "${BLUE}🚀 Deploying Kube-SlackGenie-Operator${NC}"
echo "Namespace: $NAMESPACE"
echo "Image: $IMG"
echo ""

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
echo -e "${YELLOW}📋 Checking prerequisites...${NC}"

if ! command_exists kubectl; then
    echo -e "${RED}❌ kubectl is not installed${NC}"
    exit 1
fi

if ! command_exists docker && ! command_exists podman; then
    echo -e "${RED}❌ Docker or Podman is required${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Prerequisites check passed${NC}"
echo ""

# Check if Slack webhook URL is provided
if [ -z "$SLACK_WEBHOOK_URL" ]; then
    echo -e "${YELLOW}⚠️  SLACK_WEBHOOK_URL environment variable not set${NC}"
    echo "Please set your Slack webhook URL:"
    echo "export SLACK_WEBHOOK_URL='https://hooks.slack.com/services/YOUR/WEBHOOK/URL'"
    echo ""
    read -p "Enter your Slack Webhook URL: " SLACK_WEBHOOK_URL
    
    if [ -z "$SLACK_WEBHOOK_URL" ]; then
        echo -e "${RED}❌ Slack webhook URL is required${NC}"
        exit 1
    fi
fi

# Build the operator image
echo -e "${YELLOW}🔨 Building operator image...${NC}"
make docker-build IMG=$IMG

# Deploy the operator
echo -e "${YELLOW}🚀 Deploying operator to cluster...${NC}"

# Create namespace if it doesn't exist
kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -

# Create the Slack webhook secret
echo -e "${YELLOW}🔐 Creating Slack webhook secret...${NC}"
kubectl create secret generic slack-webhook \
    --from-literal=webhook-url="$SLACK_WEBHOOK_URL" \
    --namespace=$NAMESPACE \
    --dry-run=client -o yaml | kubectl apply -f -

# Deploy the operator
make deploy IMG=$IMG

echo ""
echo -e "${GREEN}✅ Deployment completed successfully!${NC}"
echo ""
echo "📝 Next steps:"
echo "1. Check operator status:"
echo "   kubectl get pods -n $NAMESPACE"
echo ""
echo "2. View operator logs:"
echo "   kubectl logs -f deployment/controller-manager -n $NAMESPACE"
echo ""
echo "3. Test with example pods:"
echo "   kubectl apply -f examples/"
echo ""
echo "4. Monitor your Slack channel for alerts!"
echo ""
echo -e "${BLUE}🎉 Happy monitoring with Kube-SlackGenie-Operator!${NC}"
