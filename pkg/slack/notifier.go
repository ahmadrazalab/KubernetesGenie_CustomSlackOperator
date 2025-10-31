package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// SlackMessage represents the structure of a Slack webhook message
type SlackMessage struct {
	Text   string  `json:"text"`
	Color  string  `json:"color,omitempty"`
	Blocks []Block `json:"blocks,omitempty"`
}

// Block represents a Slack block kit structure
type Block struct {
	Type string     `json:"type"`
	Text *BlockText `json:"text,omitempty"`
}

// BlockText represents text within a Slack block
type BlockText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// PodAlert contains information about a pod failure
type PodAlert struct {
	PodName       string
	Namespace     string
	ContainerName string
	Image         string
	Reason        string
	Message       string
	RestartCount  int32
	Timestamp     time.Time
}

// Notifier handles Slack notifications
type Notifier struct {
	webhookURL string
	httpClient *http.Client
	logger     logr.Logger
}

// NewNotifier creates a new Slack notifier instance
func NewNotifier(logger logr.Logger) (*Notifier, error) {
	webhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		return nil, fmt.Errorf("SLACK_WEBHOOK_URL environment variable not set")
	}

	return &Notifier{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}, nil
}

// SendPodAlert sends a formatted alert message to Slack
func (n *Notifier) SendPodAlert(alert PodAlert) error {
	message := n.formatAlertMessage(alert)

	slackMsg := SlackMessage{
		Text: message,
		Blocks: []Block{
			{
				Type: "section",
				Text: &BlockText{
					Type: "mrkdwn",
					Text: message,
				},
			},
		},
	}

	jsonData, err := json.Marshal(slackMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	resp, err := n.httpClient.Post(n.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send Slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Slack webhook returned status code: %d", resp.StatusCode)
	}

	n.logger.Info("Slack alert sent successfully",
		"pod", alert.PodName,
		"namespace", alert.Namespace,
		"reason", alert.Reason,
		"restarts", alert.RestartCount,
	)

	return nil
}

// formatAlertMessage formats the pod alert into a readable Slack message
func (n *Notifier) formatAlertMessage(alert PodAlert) string {
	emoji := n.getEmojiForReason(alert.Reason)

	return fmt.Sprintf(`%s *Kube-SlackGenie Alert:*

*Pod:* %s (namespace: %s)
*Container:* %s
*Image:* %s
*Reason:* %s
*Message:* %s
*Restarts:* %d
*Time:* %s`,
		emoji,
		alert.PodName,
		alert.Namespace,
		alert.ContainerName,
		alert.Image,
		alert.Reason,
		alert.Message,
		alert.RestartCount,
		alert.Timestamp.Format(time.RFC3339),
	)
}

// getEmojiForReason returns appropriate emoji based on failure reason
func (n *Notifier) getEmojiForReason(reason string) string {
	switch reason {
	case "CrashLoopBackOff":
		return "🚨"
	case "ImagePullBackOff":
		return "🔴"
	case "ErrImagePull":
		return "📦"
	case "OOMKilled":
		return "💥"
	case "FailedScheduling":
		return "⏰"
	default:
		return "⚠️"
	}
}

// CreatePodAlertFromPod extracts alert information from a Pod resource
func CreatePodAlertFromPod(pod *corev1.Pod) *PodAlert {
	if pod == nil {
		return nil
	}

	// Find the first container with issues
	var containerName, image, reason, message string
	var restartCount int32

	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Waiting != nil {
			containerName = containerStatus.Name
			image = containerStatus.Image
			reason = containerStatus.State.Waiting.Reason
			message = containerStatus.State.Waiting.Message
			restartCount = containerStatus.RestartCount
			break
		}
		if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode != 0 {
			containerName = containerStatus.Name
			image = containerStatus.Image
			reason = containerStatus.State.Terminated.Reason
			message = containerStatus.State.Terminated.Message
			restartCount = containerStatus.RestartCount
			break
		}
	}

	// If no container status found, check init containers
	if containerName == "" {
		for _, containerStatus := range pod.Status.InitContainerStatuses {
			if containerStatus.State.Waiting != nil {
				containerName = containerStatus.Name
				image = containerStatus.Image
				reason = containerStatus.State.Waiting.Reason
				message = containerStatus.State.Waiting.Message
				restartCount = containerStatus.RestartCount
				break
			}
		}
	}

	// Fallback to pod-level information
	if containerName == "" && len(pod.Spec.Containers) > 0 {
		containerName = pod.Spec.Containers[0].Name
		image = pod.Spec.Containers[0].Image
		reason = string(pod.Status.Phase)
		message = pod.Status.Message
	}

	return &PodAlert{
		PodName:       pod.Name,
		Namespace:     pod.Namespace,
		ContainerName: containerName,
		Image:         image,
		Reason:        reason,
		Message:       message,
		RestartCount:  restartCount,
		Timestamp:     time.Now(),
	}
}
