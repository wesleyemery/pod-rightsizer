// api/v1alpha1/podrightsize_types.go

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceRecommendation contains the recommended resource values
type ResourceRecommendation struct {
	// CurrentCPURequest is the current CPU request for the container
	CurrentCPURequest string `json:"currentCPURequest,omitempty"`
	// CurrentMemoryRequest is the current memory request for the container
	CurrentMemoryRequest string `json:"currentMemoryRequest,omitempty"`
	// CurrentCPULimit is the current CPU limit for the container
	CurrentCPULimit string `json:"currentCPULimit,omitempty"`
	// CurrentMemoryLimit is the current memory limit for the container
	CurrentMemoryLimit string `json:"currentMemoryLimit,omitempty"`

	// RecommendedCPURequest is the recommended CPU request for the container
	RecommendedCPURequest string `json:"recommendedCPURequest,omitempty"`
	// RecommendedMemoryRequest is the recommended memory request for the container
	RecommendedMemoryRequest string `json:"recommendedMemoryRequest,omitempty"`
	// RecommendedCPULimit is the recommended CPU limit for the container
	RecommendedCPULimit string `json:"recommendedCPULimit,omitempty"`
	// RecommendedMemoryLimit is the recommended memory limit for the container
	RecommendedMemoryLimit string `json:"recommendedMemoryLimit,omitempty"`

	// CPUSavings is the estimated CPU savings as a percentage
	CPUSavings float64 `json:"cpuSavings,omitempty"`
	// MemorySavings is the estimated memory savings as a percentage
	MemorySavings float64 `json:"memorySavings,omitempty"`
}

// ContainerRecommendation contains recommendations for a specific container
type ContainerRecommendation struct {
	// Name is the name of the container
	Name string `json:"name"`
	// Resources contains the resource recommendations
	Resources ResourceRecommendation `json:"resources"`
}

// PodRightSizeRecommendationSpec defines the desired state of PodRightSizeRecommendation
type PodRightSizeRecommendationSpec struct {
	// TargetNamespace is the namespace to analyze pods in
	TargetNamespace string `json:"targetNamespace,omitempty"`
	
	// TargetPod is the name of a specific pod to analyze
	// +optional
	TargetPod string `json:"targetPod,omitempty"`
	
	// TargetDeployment is the name of a specific deployment to analyze
	// +optional
	TargetDeployment string `json:"targetDeployment,omitempty"`
	
	// LookbackPeriod is the duration to look back for metrics (e.g., "24h", "7d")
	LookbackPeriod string `json:"lookbackPeriod"`
	
	// CPUPercentile is the percentile to use for CPU recommendations (e.g., 95)
	CPUPercentile int `json:"cpuPercentile,omitempty"`
	
	// MemoryPercentile is the percentile to use for memory recommendations (e.g., 95)
	MemoryPercentile int `json:"memoryPercentile,omitempty"`
	
	// ApplyRecommendations indicates whether to automatically apply the recommendations
	// +optional
	ApplyRecommendations bool `json:"applyRecommendations,omitempty"`
}

// PodRightSizeRecommendationStatus defines the observed state of PodRightSizeRecommendation
type PodRightSizeRecommendationStatus struct {
	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	
	// LastAnalysisTime is the time of the last analysis
	LastAnalysisTime metav1.Time `json:"lastAnalysisTime,omitempty"`
	
	// PodRecommendations contains recommendations for each pod
	PodRecommendations []PodRecommendation `json:"podRecommendations,omitempty"`
	
	// Phase is the current phase of the recommendation
	Phase string `json:"phase,omitempty"`
}

// PodRecommendation contains recommendations for a specific pod
type PodRecommendation struct {
	// PodName is the name of the pod
	PodName string `json:"podName"`
	
	// Namespace is the namespace of the pod
	Namespace string `json:"namespace"`
	
	// OwnerKind is the kind of the owner resource (e.g., Deployment, StatefulSet)
	OwnerKind string `json:"ownerKind,omitempty"`
	
	// OwnerName is the name of the owner resource
	OwnerName string `json:"ownerName,omitempty"`
	
	// ContainerRecommendations contains recommendations for each container in the pod
	ContainerRecommendations []ContainerRecommendation `json:"containerRecommendations"`
	
	// Applied indicates whether the recommendation has been applied
	Applied bool `json:"applied,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced,shortName=prs

// PodRightSizeRecommendation is the Schema for the PodRightSizeRecommendations API
type PodRightSizeRecommendation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodRightSizeRecommendationSpec   `json:"spec,omitempty"`
	Status PodRightSizeRecommendationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodRightSizeRecommendationList contains a list of PodRightSizeRecommendation
type PodRightSizeRecommendationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodRightSizeRecommendation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodRightSizeRecommendation{}, &PodRightSizeRecommendationList{})
}

// controllers/podrightsize_controller.go

package controllers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	optimizationv1alpha1 "pod-rightsizer/api/v1alpha1"
	"pod-rightsizer/pkg/metrics"
)

// PodRightSizeRecommendationReconciler reconciles a PodRightSizeRecommendation object
type PodRightSizeRecommendationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	
	// MetricsClient is used to get pod metrics
	MetricsClient *metrics.Client
}

//+kubebuilder:rbac:groups=optimization.yourdomain.com,resources=podrightSizerecommendations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=optimization.yourdomain.com,resources=podrightSizerecommendations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=optimization.yourdomain.com,resources=podrightSizerecommendations/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=metrics.k8s.io,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PodRightSizeRecommendationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling PodRightSizeRecommendation", "request", req)

	// Fetch the PodRightSizeRecommendation instance
	prs := &optimizationv1alpha1.PodRightSizeRecommendation{}
	err := r.Get(ctx, req.NamespacedName, prs)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			logger.Info("PodRightSizeRecommendation resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get PodRightSizeRecommendation")
		return ctrl.Result{}, err
	}

	// Initialize status if it's a new resource
	if prs.Status.Phase == "" {
		prs.Status.Phase = "Pending"
		prs.Status.Conditions = []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				LastTransitionTime: metav1.Now(),
				Reason:             "Initializing",
				Message:            "Starting resource analysis",
			},
		}
		if err := r.Status().Update(ctx, prs); err != nil {
			logger.Error(err, "Failed to update PodRightSizeRecommendation status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// If already completed or failed, don't requeue
	if prs.Status.Phase == "Completed" || prs.Status.Phase == "Failed" {
		return ctrl.Result{}, nil
	}

	prs.Status.Phase = "InProgress"
	updateCondition(prs, "Ready", metav1.ConditionFalse, "InProgress", "Analyzing pod resources")
	if err := r.Status().Update(ctx, prs); err != nil {
		logger.Error(err, "Failed to update PodRightSizeRecommendation status")
		return ctrl.Result{}, err
	}

	// Analyze resources and generate recommendations
	err = r.analyzeResources(ctx, prs)
	if err != nil {
		prs.Status.Phase = "Failed"
		updateCondition(prs, "Ready", metav1.ConditionFalse, "Failed", fmt.Sprintf("Failed to analyze resources: %v", err))
		if updateErr := r.Status().Update(ctx, prs); updateErr != nil {
			logger.Error(updateErr, "Failed to update PodRightSizeRecommendation status after analysis error")
		}
		return ctrl.Result{}, err
	}

	// Apply recommendations if requested
	if prs.Spec.ApplyRecommendations {
		if err := r.applyRecommendations(ctx, prs); err != nil {
			logger.Error(err, "Failed to apply recommendations")
			updateCondition(prs, "Ready", metav1.ConditionFalse, "ApplyFailed", fmt.Sprintf("Failed to apply recommendations: %v", err))
			if updateErr := r.Status().Update(ctx, prs); updateErr != nil {
				logger.Error(updateErr, "Failed to update PodRightSizeRecommendation status after apply error")
			}
			return ctrl.Result{}, err
		}
	}

	// Update status to completed
	prs.Status.Phase = "Completed"
	prs.Status.LastAnalysisTime = metav1.Now()
	updateCondition(prs, "Ready", metav1.ConditionTrue, "Completed", "Successfully generated recommendations")
	if err := r.Status().Update(ctx, prs); err != nil {
		logger.Error(err, "Failed to update PodRightSizeRecommendation status")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciliation completed successfully")
	return ctrl.Result{}, nil
}

// analyzeResources analyzes pod resources and generates recommendations
func (r *PodRightSizeRecommendationReconciler) analyzeResources(ctx context.Context, prs *optimizationv1alpha1.PodRightSizeRecommendation) error {
	logger := log.FromContext(ctx)
	logger.Info("Analyzing resources", "namespace", prs.Spec.TargetNamespace)

	// Parse the lookback period
	lookbackDuration, err := time.ParseDuration(prs.Spec.LookbackPeriod)
	if err != nil {
		return fmt.Errorf("invalid lookback period: %v", err)
	}

	// Get all pods in the target namespace
	var pods corev1.PodList
	listOpts := []client.ListOption{
		client.InNamespace(prs.Spec.TargetNamespace),
	}
	
	// If a specific pod is targeted, add it to the filter
	if prs.Spec.TargetPod != "" {
		listOpts = append(listOpts, client.MatchingFields{
			"metadata.name": prs.Spec.TargetPod,
		})
	}

	// If a specific deployment is targeted, we'll filter the pods by owner reference later
	
	if err := r.List(ctx, &pods, listOpts...); err != nil {
		return fmt.Errorf("failed to list pods: %v", err)
	}

	// Filter pods by owner deployment if specified
	filteredPods := []corev1.Pod{}
	if prs.Spec.TargetDeployment != "" {
		for _, pod := range pods.Items {
			for _, owner := range pod.OwnerReferences {
				if owner.Kind == "ReplicaSet" {
					// Fetch the ReplicaSet to check if it's owned by the target deployment
					var rs corev1.ReplicaSet
					if err := r.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: owner.Name}, &rs); err == nil {
						for _, rsOwner := range rs.OwnerReferences {
							if rsOwner.Kind == "Deployment" && rsOwner.Name == prs.Spec.TargetDeployment {
								filteredPods = append(filteredPods, pod)
								break
							}
						}
					}
				}
			}
		}
	} else {
		filteredPods = pods.Items
	}

	if len(filteredPods) == 0 {
		return fmt.Errorf("no pods found matching the specified criteria")
	}

	// Process each pod
	podRecommendations := []optimizationv1alpha1.PodRecommendation{}
	for _, pod := range filteredPods {
		// Skip pods that are not in Running state
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Get pod metrics history
		podMetrics, err := r.MetricsClient.GetPodMetricsHistory(pod.Namespace, pod.Name, lookbackDuration)
		if err != nil {
			logger.Error(err, "Failed to get pod metrics", "pod", pod.Name)
			continue
		}

		if len(podMetrics) == 0 {
			logger.Info("No metrics available for pod", "pod", pod.Name)
			continue
		}

		// Process each container in the pod
		containerRecommendations := []optimizationv1alpha1.ContainerRecommendation{}
		for _, container := range pod.Spec.Containers {
			containerMetrics := filterContainerMetrics(podMetrics, container.Name)
			if len(containerMetrics) == 0 {
				logger.Info("No metrics available for container", "container", container.Name)
				continue
			}

			// Get current resource requests and limits
			currentCPURequest := container.Resources.Requests.Cpu().String()
			currentMemRequest := container.Resources.Requests.Memory().String()
			currentCPULimit := container.Resources.Limits.Cpu().String()
			currentMemLimit := container.Resources.Limits.Memory().String()

			// Calculate resource utilization percentiles
			cpuPercentile := prs.Spec.CPUPercentile
			if cpuPercentile == 0 {
				cpuPercentile = 95 // Default to 95th percentile
			}

			memPercentile := prs.Spec.MemoryPercentile
			if memPercentile == 0 {
				memPercentile = 95 // Default to 95th percentile
			}

			cpuUtilization := calculatePercentile(extractCPUUsage(containerMetrics), cpuPercentile)
			memUtilization := calculatePercentile(extractMemoryUsage(containerMetrics), memPercentile)

			// Add buffer (10% over the percentile)
			cpuRecommendation := resource.NewMilliQuantity(int64(float64(cpuUtilization)*1.1), resource.DecimalSI)
			memRecommendation := resource.NewQuantity(int64(float64(memUtilization)*1.1), resource.BinarySI)

			// Calculate CPU limits (2x requests)
			cpuLimitRecommendation := resource.NewMilliQuantity(int64(float64(cpuUtilization)*2.2), resource.DecimalSI)
			
			// Calculate memory limits (1.5x requests)
			memLimitRecommendation := resource.NewQuantity(int64(float64(memUtilization)*1.65), resource.BinarySI)

			// Calculate savings (if current requests are higher than recommendations)
			cpuSavings := calculateSavings(container.Resources.Requests.Cpu().MilliValue(), cpuRecommendation.MilliValue())
			memSavings := calculateSavings(container.Resources.Requests.Memory().Value(), memRecommendation.Value())

			containerRecommendations = append(containerRecommendations, optimizationv1alpha1.ContainerRecommendation{
				Name: container.Name,
				Resources: optimizationv1alpha1.ResourceRecommendation{
					CurrentCPURequest:      currentCPURequest,
					CurrentMemoryRequest:   currentMemRequest,
					CurrentCPULimit:        currentCPULimit,
					CurrentMemoryLimit:     currentMemLimit,
					RecommendedCPURequest:  cpuRecommendation.String(),
					RecommendedMemoryRequest: memRecommendation.String(),
					RecommendedCPULimit:    cpuLimitRecommendation.String(),
					RecommendedMemoryLimit: memLimitRecommendation.String(),
					CPUSavings:             cpuSavings,
					MemorySavings:          memSavings,
				},
			})
		}

		// Skip if no containers have recommendations
		if len(containerRecommendations) == 0 {
			continue
		}

		// Determine owner reference
		ownerKind := ""
		ownerName := ""
		for _, owner := range pod.OwnerReferences {
			if owner.Controller != nil && *owner.Controller {
				// For pods owned by a ReplicaSet, determine if the ReplicaSet is owned by a Deployment
				if owner.Kind == "ReplicaSet" {
					var rs corev1.ReplicaSet
					if err := r.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: owner.Name}, &rs); err == nil {
						for _, rsOwner := range rs.OwnerReferences {
							if rsOwner.Controller != nil && *rsOwner.Controller {
								ownerKind = rsOwner.Kind
								ownerName = rsOwner.Name
								break
							}
						}
					}
					if ownerKind == "" {
						ownerKind = owner.Kind
						ownerName = owner.Name
					}
				} else {
					ownerKind = owner.Kind
					ownerName = owner.Name
				}
				break
			}
		}

		podRecommendations = append(podRecommendations, optimizationv1alpha1.PodRecommendation{
			PodName:                  pod.Name,
			Namespace:                pod.Namespace,
			OwnerKind:                ownerKind,
			OwnerName:                ownerName,
			ContainerRecommendations: containerRecommendations,
			Applied:                  false,
		})
	}

	// Update status with recommendations
	prs.Status.PodRecommendations = podRecommendations
	return nil
}

// applyRecommendations applies the recommendations to the pods' owners
func (r *PodRightSizeRecommendationReconciler) applyRecommendations(ctx context.Context, prs *optimizationv1alpha1.PodRightSizeRecommendation) error {
	logger := log.FromContext(ctx)
	logger.Info("Applying recommendations")

	// Group recommendations by owner
	ownerRecommendations := make(map[string]map[string][]optimizationv1alpha1.PodRecommendation)
	for _, rec := range prs.Status.PodRecommendations {
		if rec.OwnerKind == "" || rec.OwnerName == "" {
			continue
		}

		if _, exists := ownerRecommendations[rec.OwnerKind]; !exists {
			ownerRecommendations[rec.OwnerKind] = make(map[string][]optimizationv1alpha1.PodRecommendation)
		}
		key := fmt.Sprintf("%s/%s", rec.Namespace, rec.OwnerName)
		ownerRecommendations[rec.OwnerKind][key] = append(ownerRecommendations[rec.OwnerKind][key], rec)
	}

	// Apply recommendations for each owner type
	for ownerKind, owners := range ownerRecommendations {
		for ownerKey, recommendations := range owners {
			namespace, name, found := getNamespaceAndName(ownerKey)
			if !found {
				continue
			}

			if ownerKind == "Deployment" {
				if err := r.updateDeployment(ctx, namespace, name, recommendations); err != nil {
					logger.Error(err, "Failed to update Deployment", "deployment", name, "namespace", namespace)
					continue
				}
			}
			// Add other owner kinds (StatefulSet, DaemonSet) as needed
		}
	}

	// Update the status to reflect that recommendations have been applied
	for i, rec := range prs.Status.PodRecommendations {
		prs.Status.PodRecommendations[i].Applied = true
	}

	return nil
}

// updateDeployment updates a Deployment with the recommended resource requirements
func (r *PodRightSizeRecommendationReconciler) updateDeployment(ctx context.Context, namespace, name string, recommendations []optimizationv1alpha1.PodRecommendation) error {
	logger := log.FromContext(ctx)
	logger.Info("Updating Deployment", "deployment", name, "namespace", namespace)

	// Get the deployment
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deployment); err != nil {
		return err
	}

	// Create a map of container name to recommendations
	containerRecommendations := make(map[string]optimizationv1alpha1.ResourceRecommendation)
	for _, podRec := range recommendations {
		for _, containerRec := range podRec.ContainerRecommendations {
			containerRecommendations[containerRec.Name] = containerRec.Resources
		}
	}

	// Update container resources
	for i, container := range deployment.Spec.Template.Spec.Containers {
		if rec, exists := containerRecommendations[container.Name]; exists {
			// Update container resources
			if rec.RecommendedCPURequest != "" {
				cpuRequest, err := resource.ParseQuantity(rec.RecommendedCPURequest)
				if err == nil {
					deployment.Spec.Template.Spec.Containers[i].Resources.Requests[corev1.ResourceCPU] = cpuRequest
				}
			}

			if rec.RecommendedMemoryRequest != "" {
				memRequest, err := resource.ParseQuantity(rec.RecommendedMemoryRequest)
				if err == nil {
					deployment.Spec.Template.Spec.Containers[i].Resources.Requests[corev1.ResourceMemory] = memRequest
				}
			}

			if rec.RecommendedCPULimit != "" {
				cpuLimit, err := resource.ParseQuantity(rec.RecommendedCPULimit)
				if err == nil {
					deployment.Spec.Template.Spec.Containers[i].Resources.Limits[corev1.ResourceCPU] = cpuLimit
				}
			}

			if rec.RecommendedMemoryLimit != "" {
				memLimit, err := resource.ParseQuantity(rec.RecommendedMemoryLimit)
				if err == nil {
					deployment.Spec.Template.Spec.Containers[i].Resources.Limits[corev1.ResourceMemory] = memLimit
				}
			}
		}
	}

	// Update the deployment
	if err := r.Update(ctx, deployment); err != nil {
		return err
	}

	return nil
}

// Helper function to update a condition in the status
func updateCondition(prs *optimizationv1alpha1.PodRightSizeRecommendation, conditionType string, status metav1.ConditionStatus, reason, message string) {
	for i, condition := range prs.Status.Conditions {
		if condition.Type == conditionType {
			if condition.Status != status || condition.Reason != reason || condition.Message != message {
				prs.Status.Conditions[i] = metav1.Condition{
					Type:               conditionType,
					Status:             status,
					LastTransitionTime: metav1.Now(),
					Reason:             reason,
					Message:            message,
				}
			}
			return
		}
	}

	// Condition not found, add it
	prs.Status.Conditions = append(prs.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// Helper function to get namespace and name from a key
func getNamespaceAndName(key string) (namespace, name string, found bool) {
	parts := strings.Split(key, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// Helper function to calculate savings
func calculateSavings(current, recommended int64) float64 {
	if current == 0 {
		return 0
	}
	if recommended >= current {
		return 0
	}
	return float64(current-recommended) / float64(current) * 100
}

// Helper function to filter container metrics
func filterContainerMetrics(podMetrics []metrics.PodMetric, containerName string) []metrics.ContainerMetric {
	var result []metrics.ContainerMetric
	for _, podMetric := range podMetrics {
		for _, containerMetric := range podMetric.Containers {
			if containerMetric.Name == containerName {
				result = append(result, containerMetric)
			}
		}
	}
	return result
}

// Helper function to extract CPU usage metrics
func extractCPUUsage(containerMetrics []metrics.ContainerMetric) []int64 {
	var result []int64
	for _, metric := range containerMetrics {
		result = append(result, metric.CPU.Usage)
	}
	return result
}

// Helper function to extract memory usage metrics
func extractMemoryUsage(containerMetrics []metrics.ContainerMetric) []int64 {
	var result []int64
	for _, metric := range containerMetrics {
		result = append(result, metric.Memory.Usage)
	}
	return result
}

// Helper function to calculate percentile of a slice of int64
func calculatePercentile(values []int64, percentile int) int64 {
	if len(values) == 0 {
		return 0
	}

	// Sort the values
	sort.Slice(values, func(i, j int) bool {
		return values[i] < values[j]
	})

	// Calculate the index
	index := int(math.Ceil(float64(percentile)/100.0*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}

	return values[index]
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodRightSizeRecommendationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&optimizationv1alpha1.PodRightSizeRecommendation{}).
		Complete(r)
}