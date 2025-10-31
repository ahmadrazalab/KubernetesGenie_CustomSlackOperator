# Test Pod Examples

This directory contains example pods that will trigger different failure conditions to test the Kube-SlackGenie-Operator.

## Usage

Deploy any of these examples to test the operator:

```bash
kubectl apply -f examples/crashloop-pod.yaml
kubectl apply -f examples/imagepull-pod.yaml
kubectl apply -f examples/oom-pod.yaml
```

Monitor the operator logs:

```bash
kubectl logs -f deployment/controller-manager -n kube-slackgenie-operator-system
```

Check your Slack channel for alerts.

## Cleanup

```bash
kubectl delete -f examples/
```
