/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/ahmadrazalab/kube-slackgenie-operator/pkg/slack"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	SlackNotifier  *slack.Notifier
	alertCache     map[string]time.Time
	alertCacheMux  sync.RWMutex
	debounceWindow time.Duration
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Fetch the Pod instance
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		// Pod was deleted or doesn't exist, clean up cache entry
		r.cleanupCacheEntry(req.NamespacedName.String())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if pod has failure conditions that should trigger alerts
	shouldAlert, reason := r.shouldAlertForPod(&pod)
	if !shouldAlert {
		return ctrl.Result{}, nil
	}

	// Check debouncing - avoid duplicate alerts for the same pod failure
	alertKey := fmt.Sprintf("%s/%s-%s", pod.Namespace, pod.Name, reason)
	if r.isRecentlyAlerted(alertKey) {
		logger.V(1).Info("Skipping alert due to debouncing",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"reason", reason,
		)
		return ctrl.Result{}, nil
	}

	// Create and send alert
	alert := slack.CreatePodAlertFromPod(&pod)
	if alert != nil {
		if err := r.SlackNotifier.SendPodAlert(*alert); err != nil {
			logger.Error(err, "Failed to send Slack alert",
				"pod", pod.Name,
				"namespace", pod.Namespace,
			)
			// Requeue to retry later
			return ctrl.Result{RequeueAfter: time.Minute * 5}, err
		}

		// Record alert in cache to prevent duplicates
		r.recordAlert(alertKey)

		logger.Info("Sent pod failure alert",
			"pod", pod.Name,
			"namespace", pod.Namespace,
			"reason", reason,
			"restarts", alert.RestartCount,
		)
	}

	return ctrl.Result{}, nil
}

// shouldAlertForPod determines if a pod should trigger an alert based on its status
func (r *PodReconciler) shouldAlertForPod(pod *corev1.Pod) (bool, string) {
	// Check pod phase
	if pod.Status.Phase == corev1.PodFailed {
		return true, string(pod.Status.Phase)
	}

	// Check container statuses for failure conditions
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Waiting != nil {
			reason := containerStatus.State.Waiting.Reason
			switch reason {
			case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "InvalidImageName", "ImageInspectError":
				return true, reason
			}
		}

		if containerStatus.State.Terminated != nil {
			reason := containerStatus.State.Terminated.Reason
			switch reason {
			case "OOMKilled", "Error", "ContainerCannotRun", "DeadlineExceeded":
				return true, reason
			}
		}

		// Check for high restart count
		if containerStatus.RestartCount > 0 && containerStatus.State.Waiting != nil {
			if containerStatus.State.Waiting.Reason == "CrashLoopBackOff" {
				return true, "CrashLoopBackOff"
			}
		}
	}

	// Check init container statuses
	for _, containerStatus := range pod.Status.InitContainerStatuses {
		if containerStatus.State.Waiting != nil {
			reason := containerStatus.State.Waiting.Reason
			switch reason {
			case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull":
				return true, fmt.Sprintf("InitContainer-%s", reason)
			}
		}
	}

	// Check pod conditions for scheduling failures
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
			if condition.Reason == "Unschedulable" {
				return true, "FailedScheduling"
			}
		}
	}

	return false, ""
}

// isRecentlyAlerted checks if we've recently sent an alert for this pod/reason combination
func (r *PodReconciler) isRecentlyAlerted(alertKey string) bool {
	r.alertCacheMux.RLock()
	defer r.alertCacheMux.RUnlock()

	lastAlert, exists := r.alertCache[alertKey]
	if !exists {
		return false
	}

	return time.Since(lastAlert) < r.debounceWindow
}

// recordAlert records that we've sent an alert for this pod/reason combination
func (r *PodReconciler) recordAlert(alertKey string) {
	r.alertCacheMux.Lock()
	defer r.alertCacheMux.Unlock()

	r.alertCache[alertKey] = time.Now()
}

// cleanupCacheEntry removes cache entries for deleted pods
func (r *PodReconciler) cleanupCacheEntry(podKey string) {
	r.alertCacheMux.Lock()
	defer r.alertCacheMux.Unlock()

	// Remove any cache entries that start with this pod key
	for key := range r.alertCache {
		if len(key) > len(podKey) && key[:len(podKey)] == podKey {
			delete(r.alertCache, key)
		}
	}
}

// NewPodReconciler creates a new PodReconciler with proper initialization
func NewPodReconciler(client client.Client, scheme *runtime.Scheme, notifier *slack.Notifier) *PodReconciler {
	return &PodReconciler{
		Client:         client,
		Scheme:         scheme,
		SlackNotifier:  notifier,
		alertCache:     make(map[string]time.Time),
		debounceWindow: 10 * time.Minute, // Configurable debounce window
	}
}

// SetupWithManager sets up the controller with the Manager with custom predicates
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create a predicate to filter events - only watch for status changes that might indicate failures
	podPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Alert on newly created pods that are already failing
			pod := e.Object.(*corev1.Pod)
			shouldAlert, _ := r.shouldAlertForPod(pod)
			return shouldAlert
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPod := e.ObjectOld.(*corev1.Pod)
			newPod := e.ObjectNew.(*corev1.Pod)

			// Only process if the pod status has changed
			if oldPod.ResourceVersion == newPod.ResourceVersion {
				return false
			}

			// Check if the new state warrants an alert
			shouldAlert, _ := r.shouldAlertForPod(newPod)
			return shouldAlert
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Clean up cache when pod is deleted
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(podPredicate).
		Named("pod").
		Complete(r)
}
