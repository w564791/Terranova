package services

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/models"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"gorm.io/gorm"
)

// PodRestartInfo tracks restart information for a pod with exponential backoff
type PodRestartInfo struct {
	RestartCount     int       // Number of restarts
	LastRestartTime  time.Time // Last restart time
	FirstUnhealthyAt time.Time // When the pod was first detected as unhealthy
}

// K8sDeploymentService handles K8s Pod creation and management for agent pools
// Note: Despite the name, this service now manages individual Pods instead of Deployments
// for better control over scale-down behavior (only delete idle pods)
type K8sDeploymentService struct {
	db                    *gorm.DB
	clientset             *kubernetes.Clientset
	podManager            *K8sPodManager // Pod槽位管理器
	freezeScheduleService *FreezeScheduleService
	platformConfigService *PlatformConfigService // Platform configuration service
	hostIP                string                 // Platform server IP for agents to connect back
	poolTokenService      *service.PoolTokenService
	poolIdleTimes         map[string]time.Time       // Track when each pool became idle (no tasks)
	podRestartInfo        map[string]*PodRestartInfo // Track pod restart info for backoff
	podRestartMu          sync.RWMutex               // Protect podRestartInfo map
}

// NewK8sDeploymentService creates a new K8s Deployment service
func NewK8sDeploymentService(db *gorm.DB) (*K8sDeploymentService, error) {
	// Try in-cluster config first, then fall back to kubeconfig
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build k8s config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s clientset: %w", err)
	}

	// Get HOST_IP from environment variable
	hostIP := os.Getenv("HOST_IP")
	if hostIP == "" {
		log.Printf("[K8sDeployment] Warning: HOST_IP not set, agents may not be able to connect back to platform")
	}

	// Initialize Platform Config Service
	platformConfigService := NewPlatformConfigService(db)

	// Initialize Pod Manager with platform config
	podManager := NewK8sPodManagerWithConfig(db, clientset, platformConfigService)

	return &K8sDeploymentService{
		db:                    db,
		clientset:             clientset,
		podManager:            podManager,
		freezeScheduleService: NewFreezeScheduleService(),
		platformConfigService: platformConfigService,
		hostIP:                hostIP,
		poolTokenService:      service.NewPoolTokenService(db),
		poolIdleTimes:         make(map[string]time.Time),
		podRestartInfo:        make(map[string]*PodRestartInfo),
	}, nil
}

// EnsureSecretForPool ensures a Secret exists for the pool's agent token
// Returns the secret name
// If no active token exists, automatically generates a new one
func (s *K8sDeploymentService) EnsureSecretForPool(ctx context.Context, pool *models.AgentPool) (string, error) {
	secretName := fmt.Sprintf("iac-agent-token-%s", pool.PoolID)
	namespace := "terraform"

	// Check if there's an active token for this pool
	var activeToken models.PoolToken
	hasActiveToken := s.db.WithContext(ctx).
		Where("pool_id = ? AND is_active = ?", pool.PoolID, true).
		First(&activeToken).Error == nil

	// If no active token, generate one immediately (auto-recovery from revoke)
	if !hasActiveToken {
		log.Printf("[K8sDeployment] No active token found for pool %s, auto-generating new token", pool.PoolID)

		// Generate token name
		tokenName, err := generateTokenName()
		if err != nil {
			return "", fmt.Errorf("failed to generate token name: %w", err)
		}

		// Generate new token
		token, err := s.poolTokenService.GenerateStaticToken(ctx, pool.PoolID, tokenName, "system-auto-recovery", nil)
		if err != nil {
			return "", fmt.Errorf("failed to generate token: %w", err)
		}

		// Check if secret exists
		existingSecret, err := s.clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err == nil {
			// Secret exists, update it with new token
			if existingSecret.Data == nil {
				existingSecret.Data = make(map[string][]byte)
			}
			existingSecret.Data["token"] = []byte(token.Token)

			// Update annotations
			if existingSecret.Annotations == nil {
				existingSecret.Annotations = make(map[string]string)
			}
			existingSecret.Annotations["iac-platform/token-name"] = token.TokenName
			existingSecret.Annotations["iac-platform/created-by"] = "system-auto-recovery"
			existingSecret.Annotations["iac-platform/created-at"] = token.CreatedAt.Format(time.RFC3339)

			_, err = s.clientset.CoreV1().Secrets(namespace).Update(ctx, existingSecret, metav1.UpdateOptions{})
			if err != nil {
				return "", fmt.Errorf("failed to update secret: %w", err)
			}
			log.Printf("[K8sDeployment] Updated secret %s with auto-generated token", secretName)
		} else if errors.IsNotFound(err) {
			// Secret doesn't exist, create it
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
					Labels: map[string]string{
						"app":       "iac-platform",
						"component": "agent-token",
						"pool-id":   pool.PoolID,
					},
					Annotations: map[string]string{
						"iac-platform/token-name": token.TokenName,
						"iac-platform/created-by": "system-auto-recovery",
						"iac-platform/created-at": token.CreatedAt.Format(time.RFC3339),
					},
				},
				Type: corev1.SecretTypeOpaque,
				StringData: map[string]string{
					"token": token.Token,
				},
			}

			_, err = s.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
			if err != nil {
				return "", fmt.Errorf("failed to create secret: %w", err)
			}
			log.Printf("[K8sDeployment] Created secret %s with auto-generated token", secretName)
		} else {
			return "", fmt.Errorf("failed to check secret: %w", err)
		}

		return secretName, nil
	}

	// Has active token, check if secret exists
	existingSecret, err := s.clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		// Secret exists, ensure annotations are up to date
		needsUpdate := false
		if existingSecret.Annotations == nil {
			existingSecret.Annotations = make(map[string]string)
			needsUpdate = true
		}

		// Update annotations if needed
		if existingSecret.Annotations["iac-platform/token-name"] != activeToken.TokenName {
			existingSecret.Annotations["iac-platform/token-name"] = activeToken.TokenName
			existingSecret.Annotations["iac-platform/created-by"] = func() string {
				if activeToken.CreatedBy != nil {
					return *activeToken.CreatedBy
				}
				return "system"
			}()
			existingSecret.Annotations["iac-platform/created-at"] = activeToken.CreatedAt.Format(time.RFC3339)
			needsUpdate = true
		}

		if needsUpdate {
			_, err = s.clientset.CoreV1().Secrets(namespace).Update(ctx, existingSecret, metav1.UpdateOptions{})
			if err != nil {
				log.Printf("[K8sDeployment] Warning: failed to update secret annotations: %v", err)
			} else {
				log.Printf("[K8sDeployment] Updated secret %s annotations", secretName)
			}
		}

		return secretName, nil
	}

	if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check secret existence: %w", err)
	}

	// Secret doesn't exist but we have active token, create secret
	// This shouldn't normally happen, but handle it for robustness
	log.Printf("[K8sDeployment] Secret %s doesn't exist but active token found, creating secret", secretName)

	// We need to get the actual token string, but we only have the hash
	// In this case, we need to generate a new token
	tokenName, err := generateTokenName()
	if err != nil {
		return "", fmt.Errorf("failed to generate token name: %w", err)
	}

	token, err := s.poolTokenService.GenerateStaticToken(ctx, pool.PoolID, tokenName, "system-secret-recovery", nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "iac-platform",
				"component": "agent-token",
				"pool-id":   pool.PoolID,
			},
			Annotations: map[string]string{
				"iac-platform/token-name": token.TokenName,
				"iac-platform/created-by": "system-secret-recovery",
				"iac-platform/created-at": token.CreatedAt.Format(time.RFC3339),
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"token": token.Token,
		},
	}

	_, err = s.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			log.Printf("[K8sDeployment] Secret %s already exists (race condition)", secretName)
			return secretName, nil
		}
		return "", fmt.Errorf("failed to create secret: %w", err)
	}

	log.Printf("[K8sDeployment] Successfully created secret %s in namespace %s", secretName, namespace)
	return secretName, nil
}

// rotateSecret rotates the agent token secret
func (s *K8sDeploymentService) rotateSecret(ctx context.Context, pool *models.AgentPool, secretName, namespace string) error {
	// 1. Revoke all existing active tokens
	var existingTokens []models.PoolToken
	if err := s.db.WithContext(ctx).
		Where("pool_id = ? AND is_active = ?", pool.PoolID, true).
		Find(&existingTokens).Error; err != nil {
		log.Printf("[K8sDeployment] Warning: failed to query existing tokens: %v", err)
	}

	for _, existingToken := range existingTokens {
		log.Printf("[K8sDeployment] Revoking existing token %s during rotation", existingToken.TokenName)
		if err := s.poolTokenService.RevokeToken(ctx, pool.PoolID, existingToken.TokenName, "system-rotation"); err != nil {
			log.Printf("[K8sDeployment] Warning: failed to revoke token %s: %v", existingToken.TokenName, err)
		}
	}

	// 2. Generate new token with format: tn-{16位随机数}
	tokenName, err := generateTokenName()
	if err != nil {
		return fmt.Errorf("failed to generate token name: %w", err)
	}

	token, err := s.poolTokenService.GenerateStaticToken(ctx, pool.PoolID, tokenName, "system-rotation", nil)
	if err != nil {
		return fmt.Errorf("failed to generate new token: %w", err)
	}

	// 3. Update secret
	secret, err := s.clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	// Update Data field (not StringData, which is only for creation)
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["token"] = []byte(token.Token)

	// Update annotations with new token info
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations["iac-platform/token-name"] = token.TokenName
	secret.Annotations["iac-platform/rotated-by"] = "system-rotation"
	secret.Annotations["iac-platform/rotated-at"] = time.Now().Format(time.RFC3339)

	_, err = s.clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	log.Printf("[K8sDeployment] Successfully rotated secret %s with new token", secretName)
	return nil
}

// EnsurePodsForPool ensures Pods exist for the given K8s agent pool
// This replaces EnsureDeploymentForPool - now manages individual Pods instead of Deployments
// When called after config update, it will recreate all idle Pods with new configuration
func (s *K8sDeploymentService) EnsurePodsForPool(ctx context.Context, pool *models.AgentPool) error {
	if pool.PoolType != models.AgentPoolTypeK8s {
		return fmt.Errorf("pool %s is not a K8s pool", pool.PoolID)
	}

	// Parse K8s config
	if pool.K8sConfig == nil {
		return fmt.Errorf("pool %s does not have K8s configuration", pool.PoolID)
	}

	var k8sConfig models.K8sJobTemplateConfig
	if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err != nil {
		return fmt.Errorf("failed to parse K8s config: %w", err)
	}

	// 1. Ensure secret exists first
	secretName, err := s.EnsureSecretForPool(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to ensure secret: %w", err)
	}

	// 2. Sync existing Pods from K8s
	if err := s.podManager.SyncPodsFromK8s(ctx, pool.PoolID); err != nil {
		log.Printf("[K8sPodService] Warning: failed to sync pods from K8s: %v", err)
	}

	// 2.5. Immediately reconcile Pods to sync task status to slots
	// This is critical on server restart to prevent deleting Pods with reserved slots
	if err := s.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
		log.Printf("[K8sPodService] Warning: failed to reconcile pods: %v", err)
	} else {
		log.Printf("[K8sPodService] Successfully reconciled pods for pool %s", pool.PoolID)
	}

	// 3. Get current Pod count
	currentCount := s.podManager.GetPodCount(pool.PoolID)
	log.Printf("[K8sPodService] Pool %s currently has %d pods", pool.PoolID, currentCount)

	// 4. Note: We DON'T delete idle Pods on server restart
	// Reason: This would cause unnecessary Pod churn and lose agent state
	// Configuration updates should be handled separately (e.g., via a dedicated API endpoint)
	// For now, we only ensure minimum Pods exist

	// 5. Ensure minimum number of Pods (based on min_replicas)
	minReplicas := k8sConfig.MinReplicas
	if minReplicas < 0 {
		minReplicas = 0 // Default to 0 if not configured
	}

	// 6. Create Pods with new configuration
	for currentCount < minReplicas {
		_, err := s.podManager.CreatePod(ctx, pool.PoolID, &k8sConfig, secretName)
		if err != nil {
			log.Printf("[K8sPodService] Failed to create pod for pool %s: %v", pool.PoolID, err)
			return fmt.Errorf("failed to create pod: %w", err)
		}
		currentCount++
		log.Printf("[K8sPodService] Created pod with new config for pool %s (now %d/%d)", pool.PoolID, currentCount, minReplicas)
	}

	log.Printf("[K8sPodService] Successfully ensured %d pods for pool %s with updated configuration", currentCount, pool.PoolID)
	return nil
}

// EnsureDeploymentForPool is deprecated - kept for backward compatibility
// Use EnsurePodsForPool instead
func (s *K8sDeploymentService) EnsureDeploymentForPool(ctx context.Context, pool *models.AgentPool) error {
	log.Printf("[K8sDeployment] DEPRECATED: EnsureDeploymentForPool called, redirecting to EnsurePodsForPool")
	return s.EnsurePodsForPool(ctx, pool)
}

// UpdateDeploymentConfig updates an existing deployment with new configuration
func (s *K8sDeploymentService) UpdateDeploymentConfig(ctx context.Context, pool *models.AgentPool, existingDeployment *appsv1.Deployment) error {
	// Parse K8s config
	var k8sConfig models.K8sJobTemplateConfig
	if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err != nil {
		return fmt.Errorf("failed to parse K8s config: %w", err)
	}

	// Keep current replica count
	currentReplicas := int32(0)
	if existingDeployment.Spec.Replicas != nil {
		currentReplicas = *existingDeployment.Spec.Replicas
	}

	// Ensure secret exists
	secretName, err := s.EnsureSecretForPool(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to ensure secret: %w", err)
	}

	// Build new deployment spec with current replica count
	newDeployment := s.buildDeployment(existingDeployment.Name, existingDeployment.Namespace, pool, &k8sConfig, currentReplicas, secretName)

	// Preserve the existing deployment's metadata (like UID, ResourceVersion)
	newDeployment.ObjectMeta.ResourceVersion = existingDeployment.ObjectMeta.ResourceVersion
	newDeployment.ObjectMeta.UID = existingDeployment.ObjectMeta.UID

	// Update the deployment
	_, err = s.clientset.AppsV1().Deployments(existingDeployment.Namespace).Update(ctx, newDeployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	log.Printf("[K8sDeployment] Successfully updated deployment %s with new configuration (replicas: %d)", existingDeployment.Name, currentReplicas)
	return nil
}

// ScalePods scales the number of Pods for a pool
// This replaces ScaleDeployment - now manages individual Pods instead of Deployment replicas
// Scale-up: Creates new Pods
// Scale-down: Only deletes Pods where ALL slots are idle (safe scale-down)
func (s *K8sDeploymentService) ScalePods(ctx context.Context, poolID string, desiredCount int) error {
	// 1. Get current Pod count
	currentCount := s.podManager.GetPodCount(poolID)

	log.Printf("[K8sPodService] Scaling pool %s from %d to %d pods", poolID, currentCount, desiredCount)

	// 2. If need to scale up
	if desiredCount > currentCount {
		// Get pool configuration
		var pool models.AgentPool
		if err := s.db.First(&pool, "pool_id = ?", poolID).Error; err != nil {
			return fmt.Errorf("failed to get pool: %w", err)
		}

		var k8sConfig models.K8sJobTemplateConfig
		if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err != nil {
			return fmt.Errorf("failed to parse K8s config: %w", err)
		}

		// Ensure secret exists
		secretName, err := s.EnsureSecretForPool(ctx, &pool)
		if err != nil {
			return fmt.Errorf("failed to ensure secret: %w", err)
		}

		// Create new Pods
		podsToCreate := desiredCount - currentCount
		for i := 0; i < podsToCreate; i++ {
			_, err := s.podManager.CreatePod(ctx, poolID, &k8sConfig, secretName)
			if err != nil {
				log.Printf("[K8sPodService] Failed to create pod %d/%d for pool %s: %v", i+1, podsToCreate, poolID, err)
				return fmt.Errorf("failed to create pod: %w", err)
			}
			log.Printf("[K8sPodService] Created pod %d/%d for pool %s", i+1, podsToCreate, poolID)
		}

		log.Printf("[K8sPodService] Successfully scaled up pool %s to %d pods", poolID, desiredCount)
		return nil
	}

	// 3. If need to scale down
	if desiredCount < currentCount {
		// Only delete completely idle Pods (all slots are idle)
		idlePods := s.podManager.FindIdlePods(poolID)
		podsToDelete := currentCount - desiredCount

		if len(idlePods) < podsToDelete {
			log.Printf("[K8sPodService] Warning: pool %s needs to scale down by %d pods, but only %d pods are completely idle",
				poolID, podsToDelete, len(idlePods))
			podsToDelete = len(idlePods)
		}

		// Delete idle Pods
		for i := 0; i < podsToDelete; i++ {
			err := s.podManager.DeletePod(ctx, idlePods[i].PodName)
			if err != nil {
				log.Printf("[K8sPodService] Failed to delete pod %s: %v", idlePods[i].PodName, err)
				return fmt.Errorf("failed to delete pod: %w", err)
			}
			log.Printf("[K8sPodService] Deleted idle pod %s (%d/%d)", idlePods[i].PodName, i+1, podsToDelete)
		}

		finalCount := s.podManager.GetPodCount(poolID)
		log.Printf("[K8sPodService] Successfully scaled down pool %s to %d pods (target was %d, deleted %d idle pods)",
			poolID, finalCount, desiredCount, podsToDelete)
		return nil
	}

	// No scaling needed
	log.Printf("[K8sPodService] Pool %s already at desired count %d, no scaling needed", poolID, desiredCount)
	return nil
}

// ScaleDeployment is deprecated - kept for backward compatibility
// Use ScalePods instead
func (s *K8sDeploymentService) ScaleDeployment(ctx context.Context, poolID string, replicas int32) error {
	log.Printf("[K8sDeployment] DEPRECATED: ScaleDeployment called, redirecting to ScalePods")
	return s.ScalePods(ctx, poolID, int(replicas))
}

// GetPodCount returns the current and desired Pod count for a pool
// This replaces GetDeploymentReplicas - now returns Pod counts instead of Deployment replicas
func (s *K8sDeploymentService) GetPodCount(ctx context.Context, poolID string) (current, desired int, err error) {
	// 1. Sync Pods from K8s to ensure we have latest state
	if err := s.podManager.SyncPodsFromK8s(ctx, poolID); err != nil {
		log.Printf("[K8sPodService] Warning: failed to sync pods: %v", err)
	}

	// 2. Get current Pod count from PodManager
	current = s.podManager.GetPodCount(poolID)

	// 3. Get desired count from pool configuration
	var pool models.AgentPool
	if err := s.db.First(&pool, "pool_id = ?", poolID).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to get pool: %w", err)
	}

	if pool.K8sConfig != nil {
		var k8sConfig models.K8sJobTemplateConfig
		if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err == nil {
			// Desired count is based on current workload, but we return current as a baseline
			// The actual desired count is calculated by AutoScalePods
			desired = current
		}
	}

	return current, desired, nil
}

// GetDeploymentReplicas is deprecated - kept for backward compatibility
// Use GetPodCount instead
func (s *K8sDeploymentService) GetDeploymentReplicas(ctx context.Context, poolID string) (current, desired int32, err error) {
	log.Printf("[K8sDeployment] DEPRECATED: GetDeploymentReplicas called, redirecting to GetPodCount")
	c, d, e := s.GetPodCount(ctx, poolID)
	return int32(c), int32(d), e
}

// DeleteDeployment deletes the deployment for a pool
func (s *K8sDeploymentService) DeleteDeployment(ctx context.Context, poolID string) error {
	deploymentName := fmt.Sprintf("iac-agent-%s", poolID)
	namespace := "terraform"

	propagationPolicy := metav1.DeletePropagationBackground
	err := s.clientset.AppsV1().Deployments(namespace).Delete(ctx, deploymentName, metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})

	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	log.Printf("[K8sDeployment] Deleted deployment %s from namespace %s", deploymentName, namespace)
	return nil
}

// buildDeployment constructs a K8s Deployment object
func (s *K8sDeploymentService) buildDeployment(deploymentName, namespace string, pool *models.AgentPool, config *models.K8sJobTemplateConfig, replicas int32, secretName string) *appsv1.Deployment {
	// Image pull policy
	imagePullPolicy := corev1.PullIfNotPresent
	if config.ImagePullPolicy != "" {
		imagePullPolicy = corev1.PullPolicy(config.ImagePullPolicy)
	}

	// Build environment variables
	envVars := []corev1.EnvVar{
		{Name: "POOL_ID", Value: pool.PoolID},
		{Name: "POOL_NAME", Value: pool.Name},
		{Name: "POOL_TYPE", Value: "k8s"},
		// IAC_AGENT_NAME will use pod hostname (auto-injected by K8s)
		{
			Name: "IAC_AGENT_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		// IAC_AGENT_TOKEN from secret
		{
			Name: "IAC_AGENT_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: "token",
				},
			},
		},
	}

	// Add custom env vars from config (including CC_SERVER_PORT, SERVER_PORT if configured)
	for key, value := range config.Env {
		envVars = append(envVars, corev1.EnvVar{Name: key, Value: value})
	}

	// Build resource requirements
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	// Parse CPU and memory limits
	if config.Resources != nil {
		if config.Resources.Limits != nil {
			if cpu, ok := config.Resources.Limits["cpu"]; ok {
				if quantity, err := resource.ParseQuantity(cpu); err == nil {
					resources.Limits[corev1.ResourceCPU] = quantity
				}
			}
			if memory, ok := config.Resources.Limits["memory"]; ok {
				if quantity, err := resource.ParseQuantity(memory); err == nil {
					resources.Limits[corev1.ResourceMemory] = quantity
				}
			}
		}
		if config.Resources.Requests != nil {
			if cpu, ok := config.Resources.Requests["cpu"]; ok {
				if quantity, err := resource.ParseQuantity(cpu); err == nil {
					resources.Requests[corev1.ResourceCPU] = quantity
				}
			}
			if memory, ok := config.Resources.Requests["memory"]; ok {
				if quantity, err := resource.ParseQuantity(memory); err == nil {
					resources.Requests[corev1.ResourceMemory] = quantity
				}
			}
		}
	}

	// Build container
	container := corev1.Container{
		Name:            "agent",
		Image:           config.Image,
		ImagePullPolicy: imagePullPolicy,
		Env:             envVars,
		Resources:       resources,
	}

	if len(config.Command) > 0 {
		container.Command = config.Command
	}
	if len(config.Args) > 0 {
		container.Args = config.Args
	}

	// Build Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "iac-platform",
				"component": "agent",
				"pool-id":   pool.PoolID,
				"pool-name": pool.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "iac-platform",
					"component": "agent",
					"pool-id":   pool.PoolID,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "iac-platform",
						"component": "agent",
						"pool-id":   pool.PoolID,
						"pool-name": pool.Name,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers:    []corev1.Container{container},
				},
			},
		},
	}

	return deployment
}

// CountPendingTasksForPool counts the number of active tasks for a specific pool
// and calculates the required number of agents based on agent capacity
//
// Agent capacity design:
// - Each agent can handle: 3 plan tasks + 1 plan_and_apply task
// - Plan tasks can run concurrently (up to 3 per agent)
// - Plan_and_apply tasks require exclusive agent (1 per agent)
//
// FIX: Only count RUNNING tasks for scaling up, not pending tasks
// Pending tasks may be blocked by workspace locks or other constraints,
// not necessarily waiting for available agents
func (s *K8sDeploymentService) CountPendingTasksForPool(ctx context.Context, poolID string) (int64, error) {
	// Count plan tasks (ONLY running, not pending)
	// Pending tasks might be blocked by workspace locks, not lack of agents
	var planTaskCount int64
	err := s.db.WithContext(ctx).
		Model(&models.WorkspaceTask{}).
		Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
		Where("workspaces.current_pool_id = ?", poolID).
		Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
		Where("workspace_tasks.task_type = ?", models.TaskTypePlan).
		Where("workspace_tasks.status = ?", models.TaskStatusRunning).
		Count(&planTaskCount).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count plan tasks: %w", err)
	}

	// Count plan_and_apply tasks (running + apply_pending)
	// apply_pending tasks need agents to stay alive for apply execution after user confirmation
	// Do NOT count pending tasks - they may be blocked by other constraints
	var planAndApplyTaskCount int64
	err = s.db.WithContext(ctx).
		Model(&models.WorkspaceTask{}).
		Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
		Where("workspaces.current_pool_id = ?", poolID).
		Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
		Where("workspace_tasks.task_type = ?", models.TaskTypePlanAndApply).
		Where("workspace_tasks.status IN (?)", []models.TaskStatus{
			models.TaskStatusRunning,
			models.TaskStatusApplyPending,
		}).
		Count(&planAndApplyTaskCount).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count plan_and_apply tasks: %w", err)
	}

	// Calculate required agents based on capacity:
	// - Plan tasks: 3 per agent (round up)
	// - Plan_and_apply tasks: 1 per agent
	// Required agents = max(ceil(plan_tasks / 3), plan_and_apply_tasks)

	agentsForPlanTasks := (planTaskCount + 2) / 3 // Ceiling division
	agentsForPlanAndApplyTasks := planAndApplyTaskCount

	requiredAgents := agentsForPlanTasks
	if agentsForPlanAndApplyTasks > requiredAgents {
		requiredAgents = agentsForPlanAndApplyTasks
	}

	log.Printf("[K8sDeployment] Pool %s capacity calculation: plan_tasks=%d (running only), plan_and_apply_tasks=%d (running+apply_pending), agents_for_plan=%d, agents_for_plan_and_apply=%d, required_agents=%d",
		poolID, planTaskCount, planAndApplyTaskCount, agentsForPlanTasks, agentsForPlanAndApplyTasks, requiredAgents)

	return requiredAgents, nil
}

// AutoScalePods performs auto-scaling logic for a pool's Pods based on slot utilization
// This replaces AutoScaleDeployment - now uses slot-based capacity planning
// Returns the new Pod count and whether scaling was performed
//
// Slot-based scaling strategy:
// - Scale up: When slot utilization > 80% or has reserved slots
// - Scale down: When slot utilization < 20% and no reserved slots
// - Only delete completely idle Pods (all slots idle)
// - Respect reserved slots (apply_pending tasks)
// - Freeze window: Scale down to 0 (only delete idle Pods)
// - One-time unfreeze: Restore to min_replicas (default 1)
func (s *K8sDeploymentService) AutoScalePods(ctx context.Context, pool *models.AgentPool) (int, bool, error) {
	// Parse K8s config
	if pool.K8sConfig == nil {
		return 0, false, fmt.Errorf("pool %s does not have K8s configuration", pool.PoolID)
	}

	var k8sConfig models.K8sJobTemplateConfig
	if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err != nil {
		return 0, false, fmt.Errorf("failed to parse K8s config: %w", err)
	}

	// Check freeze schedule (with one-time unfreeze support)
	inFreeze, reason := s.freezeScheduleService.IsInFreezeWindowWithUnfreeze(k8sConfig.FreezeSchedules, pool.OneTimeUnfreezeUntil)
	if inFreeze {
		// In freeze window: scale down to 0 (only delete idle Pods to protect running tasks)
		log.Printf("[K8sPodService] Pool %s is in freeze window: %s, scaling down to 0", pool.PoolID, reason)

		// Reconcile Pods first to ensure state is up-to-date
		if err := s.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
			log.Printf("[K8sPodService] Warning: failed to reconcile pods: %v", err)
		}

		currentPodCount := s.podManager.GetPodCount(pool.PoolID)
		if currentPodCount == 0 {
			log.Printf("[K8sPodService] Pool %s already at 0 pods during freeze window", pool.PoolID)
			return 0, false, nil
		}

		// Find and delete only idle Pods (protect Pods with running/reserved tasks)
		idlePods := s.podManager.FindIdlePods(pool.PoolID)
		if len(idlePods) == 0 {
			log.Printf("[K8sPodService] Pool %s has %d pods but none are idle during freeze window (tasks still running)",
				pool.PoolID, currentPodCount)
			return currentPodCount, false, nil
		}

		// Delete all idle Pods
		deletedCount := 0
		for _, pod := range idlePods {
			if err := s.podManager.DeletePod(ctx, pod.PodName); err != nil {
				log.Printf("[K8sPodService] Warning: failed to delete idle pod %s during freeze: %v", pod.PodName, err)
			} else {
				deletedCount++
				log.Printf("[K8sPodService] Deleted idle pod %s during freeze window (%d/%d)",
					pod.PodName, deletedCount, len(idlePods))
			}
		}

		finalCount := s.podManager.GetPodCount(pool.PoolID)
		if deletedCount > 0 {
			log.Printf("[K8sPodService] Pool %s freeze window: deleted %d idle pods, %d pods remaining (with running tasks)",
				pool.PoolID, deletedCount, finalCount)
			return finalCount, true, nil
		}

		return finalCount, false, nil
	}

	// 1. Reconcile Pods to ensure state is up-to-date
	if err := s.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
		log.Printf("[K8sPodService] Warning: failed to reconcile pods: %v", err)
	}

	// 2. Get current Pod count and slot statistics
	currentPodCount := s.podManager.GetPodCount(pool.PoolID)
	totalSlots, usedSlots, reservedSlots, idleSlots := s.podManager.GetSlotStats(pool.PoolID)

	log.Printf("[K8sPodService] Pool %s status: pods=%d, slots(total=%d, used=%d, reserved=%d, idle=%d)",
		pool.PoolID, currentPodCount, totalSlots, usedSlots, reservedSlots, idleSlots)

	// 3. Calculate slot utilization
	var utilizationRate float64
	if totalSlots > 0 {
		utilizationRate = float64(usedSlots+reservedSlots) / float64(totalSlots)
	}

	// 4. Determine desired Pod count based on slot utilization
	var desiredPodCount int

	if totalSlots == 0 || currentPodCount == 0 {
		// Cold start scenario - simplified logic
		// Simple check: do we have ANY pending tasks?
		var pendingTaskCount int64
		err := s.db.WithContext(ctx).
			Model(&models.WorkspaceTask{}).
			Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
			Where("workspaces.current_pool_id = ?", pool.PoolID).
			Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
			Where("workspace_tasks.status = ?", models.TaskStatusPending).
			Count(&pendingTaskCount).Error

		if err != nil {
			log.Printf("[K8sPodService] Error checking pending tasks for pool %s: %v", pool.PoolID, err)
			// On error, respect min_replicas to ensure availability
			desiredPodCount = k8sConfig.MinReplicas
			log.Printf("[K8sPodService] Pool %s cold start: query error, defaulting to min_replicas=%d",
				pool.PoolID, desiredPodCount)
		} else if pendingTaskCount > 0 {
			// Has pending tasks, start with min_replicas or 1
			desiredPodCount = k8sConfig.MinReplicas
			if desiredPodCount < 1 {
				desiredPodCount = 1
			}
			log.Printf("[K8sPodService] Pool %s cold start: %d pending tasks, scaling to %d pods",
				pool.PoolID, pendingTaskCount, desiredPodCount)
		} else {
			// No pending tasks, respect min_replicas
			desiredPodCount = k8sConfig.MinReplicas
			log.Printf("[K8sPodService] Pool %s cold start: no pending tasks, scaling to min_replicas=%d",
				pool.PoolID, desiredPodCount)
		}
	} else {
		// 4.5. 特殊检查：是否有不被 block 的 pending plan_and_apply 任务但所有 Pod 都被占用
		// 只检查每个 workspace 的第一个 pending 任务（不被其他任务 block 的）
		// 这种情况需要创建新 Pod，即使槽位利用率不高
		var unblockedPendingPlanAndApplyCount int64
		err := s.db.WithContext(ctx).
			Table("workspace_tasks AS wt1").
			Joins("JOIN workspaces ON workspaces.workspace_id = wt1.workspace_id").
			Where("workspaces.current_pool_id = ?", pool.PoolID).
			Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
			Where("wt1.status = ?", models.TaskStatusPending).
			Where("wt1.task_type = ?", models.TaskTypePlanAndApply).
			Where(`NOT EXISTS (
				SELECT 1 FROM workspace_tasks AS wt2 
				WHERE wt2.workspace_id = wt1.workspace_id 
				AND wt2.id < wt1.id 
				AND wt2.status IN ('pending', 'running', 'apply_pending')
			)`).
			Count(&unblockedPendingPlanAndApplyCount).Error

		if err != nil {
			log.Printf("[K8sPodService] Warning: failed to check unblocked pending plan_and_apply tasks: %v", err)
		} else if unblockedPendingPlanAndApplyCount > 0 {
			log.Printf("[K8sPodService] Pool %s has %d unblocked pending plan_and_apply tasks",
				pool.PoolID, unblockedPendingPlanAndApplyCount)
			// 计算需要的 Pod 数量
			// 每个 running 的 plan_and_apply 任务需要 1 个 Pod
			// 每个 unblocked pending 的 plan_and_apply 任务也需要 1 个 Pod

			// 统计有 running 任务的 Pod 数量
			pods := s.podManager.ListPods(pool.PoolID)
			podsWithRunningTasks := 0
			for _, pod := range pods {
				pod.mu.RLock()
				hasRunning := false
				for _, slot := range pod.Slots {
					if slot.Status == "running" {
						hasRunning = true
						break
					}
				}
				pod.mu.RUnlock()

				if hasRunning {
					podsWithRunningTasks++
				}
			}

			// 需要的总 Pod 数 = 有 running 任务的 Pod 数 + unblocked pending 任务数
			requiredPods := podsWithRunningTasks + int(unblockedPendingPlanAndApplyCount)

			if requiredPods > currentPodCount {
				desiredPodCount = requiredPods
				if desiredPodCount > k8sConfig.MaxReplicas {
					desiredPodCount = k8sConfig.MaxReplicas
				}
				log.Printf("[K8sPodService] Pool %s needs %d pods (running_pods=%d + unblocked_pending=%d), current=%d, scaling to %d",
					pool.PoolID, requiredPods, podsWithRunningTasks, unblockedPendingPlanAndApplyCount, currentPodCount, desiredPodCount)

				// 跳过后续的利用率检查，直接执行扩容
				goto performScaling
			}
		}
	}

	if reservedSlots > 0 {
		// Has reserved slots (apply_pending tasks) - must keep these Pods
		// Calculate minimum Pods needed for reserved slots
		minPodsForReserved := (reservedSlots + 2) / 3 // Each Pod has 3 slots
		desiredPodCount = currentPodCount
		if desiredPodCount < minPodsForReserved {
			desiredPodCount = minPodsForReserved
		}

		// Additional check: ensure all apply_pending tasks have slots
		// Note: We DON'T trigger scale-up for apply_pending tasks without slots
		// Reason: apply_pending tasks should wait for their reserved slot Pod to come back online
		// Only if the Pod is truly deleted (not just offline), the task will need to be re-executed from plan
		// The current implementation with retry + fallback handles this correctly:
		// - If reserved Pod exists but offline: task will retry and wait
		// - If reserved Pod is deleted: task will fallback to new Pod and re-execute from plan

		// However, we log a warning if apply_pending tasks have no slots for monitoring
		var applyPendingTasks []models.WorkspaceTask
		err := s.db.WithContext(ctx).
			Model(&models.WorkspaceTask{}).
			Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
			Where("workspaces.current_pool_id = ?", pool.PoolID).
			Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
			Where("workspace_tasks.status = ?", models.TaskStatusApplyPending).
			Find(&applyPendingTasks).Error

		if err != nil {
			log.Printf("[K8sPodService] Warning: failed to query apply_pending tasks: %v", err)
		} else if len(applyPendingTasks) > 0 {
			// Check if each apply_pending task has a slot
			tasksWithoutSlot := 0
			for _, task := range applyPendingTasks {
				_, _, err := s.podManager.FindPodByTaskID(task.ID)
				if err != nil {
					tasksWithoutSlot++
					log.Printf("[K8sPodService] Warning: Apply_pending task %d has no slot (Pod may be offline or deleted)", task.ID)
				}
			}

			if tasksWithoutSlot > 0 {
				log.Printf("[K8sPodService] Pool %s has %d apply_pending tasks without slots (will wait for Pod to come back or retry with fallback)",
					pool.PoolID, tasksWithoutSlot)
			}
		}

		log.Printf("[K8sPodService] Pool %s has %d reserved slots, desired pod count: %d",
			pool.PoolID, reservedSlots, desiredPodCount)
	} else if utilizationRate > 0.8 {
		// High utilization (>80%) - scale up
		desiredPodCount = currentPodCount + 1
		if desiredPodCount > k8sConfig.MaxReplicas {
			desiredPodCount = k8sConfig.MaxReplicas
		}
		log.Printf("[K8sPodService] Pool %s high utilization (%.1f%%), scaling up to %d pods",
			pool.PoolID, utilizationRate*100, desiredPodCount)
	} else if utilizationRate < 0.2 && usedSlots == 0 {
		// Low utilization (<20%) and no active tasks - but check for pending tasks first
		// 检查是否有不被 block 的 pending 任务
		var unblockedPendingCount int64
		err := s.db.WithContext(ctx).
			Table("workspace_tasks AS wt1").
			Joins("JOIN workspaces ON workspaces.workspace_id = wt1.workspace_id").
			Where("workspaces.current_pool_id = ?", pool.PoolID).
			Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
			Where("wt1.status = ?", models.TaskStatusPending).
			Where(`NOT EXISTS (
				SELECT 1 FROM workspace_tasks AS wt2 
				WHERE wt2.workspace_id = wt1.workspace_id 
				AND wt2.id < wt1.id 
				AND wt2.status IN ('pending', 'running', 'apply_pending')
			)`).
			Count(&unblockedPendingCount).Error

		if err != nil {
			log.Printf("[K8sPodService] Warning: failed to check unblocked pending tasks: %v", err)
			// On error, don't scale down to be safe
			desiredPodCount = currentPodCount
		} else if unblockedPendingCount > 0 {
			// Has unblocked pending tasks, don't scale down
			desiredPodCount = currentPodCount
			log.Printf("[K8sPodService] Pool %s has low utilization but %d unblocked pending tasks, maintaining %d pods",
				pool.PoolID, unblockedPendingCount, currentPodCount)
		} else {
			// No pending tasks, safe to scale down
			desiredPodCount = currentPodCount - 1
			if desiredPodCount < k8sConfig.MinReplicas {
				desiredPodCount = k8sConfig.MinReplicas
			}
			log.Printf("[K8sPodService] Pool %s low utilization (%.1f%%) and no pending tasks, scaling down to %d pods",
				pool.PoolID, utilizationRate*100, desiredPodCount)
		}
	} else {
		// Normal utilization - maintain current count
		desiredPodCount = currentPodCount
	}

performScaling:
	// 5. Respect min/max constraints
	if desiredPodCount < k8sConfig.MinReplicas {
		desiredPodCount = k8sConfig.MinReplicas
	}
	if desiredPodCount > k8sConfig.MaxReplicas {
		desiredPodCount = k8sConfig.MaxReplicas
	}

	// 6. Only scale if there's a change
	if desiredPodCount == currentPodCount {
		return currentPodCount, false, nil
	}

	// 6.5. Additional protection: check for apply_pending tasks before scale-down
	// This prevents deleting Pods during server restart before slots are properly synced
	if desiredPodCount < currentPodCount {
		var applyPendingCount int64
		err := s.db.WithContext(ctx).
			Model(&models.WorkspaceTask{}).
			Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
			Where("workspaces.current_pool_id = ?", pool.PoolID).
			Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
			Where("workspace_tasks.status = ?", models.TaskStatusApplyPending).
			Count(&applyPendingCount).Error

		if err != nil {
			log.Printf("[K8sPodService] Warning: failed to check apply_pending tasks: %v", err)
		} else if applyPendingCount > 0 {
			log.Printf("[K8sPodService] Pool %s has %d apply_pending tasks, skipping scale-down to protect reserved slots",
				pool.PoolID, applyPendingCount)
			return currentPodCount, false, nil
		}
	}

	// 7. Perform scaling
	log.Printf("[K8sPodService] Scaling pool %s from %d to %d pods (utilization: %.1f%%)",
		pool.PoolID, currentPodCount, desiredPodCount, utilizationRate*100)

	if err := s.ScalePods(ctx, pool.PoolID, desiredPodCount); err != nil {
		return 0, false, fmt.Errorf("failed to scale pods: %w", err)
	}

	return desiredPodCount, true, nil
}

// AutoScaleDeployment is deprecated - kept for backward compatibility
// Use AutoScalePods instead
func (s *K8sDeploymentService) AutoScaleDeployment(ctx context.Context, pool *models.AgentPool) (int32, bool, error) {
	log.Printf("[K8sDeployment] DEPRECATED: AutoScaleDeployment called, redirecting to AutoScalePods")
	count, scaled, err := s.AutoScalePods(ctx, pool)
	return int32(count), scaled, err
}

// AutoScaleDeployment_OLD is the old implementation - kept for reference during migration
// TODO: Remove this after migration is complete and tested
func (s *K8sDeploymentService) AutoScaleDeployment_OLD(ctx context.Context, pool *models.AgentPool) (int32, bool, error) {
	// Parse K8s config
	if pool.K8sConfig == nil {
		return 0, false, fmt.Errorf("pool %s does not have K8s configuration", pool.PoolID)
	}

	var k8sConfig models.K8sJobTemplateConfig
	if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err != nil {
		return 0, false, fmt.Errorf("failed to parse K8s config: %w", err)
	}

	// Check freeze schedule (with one-time unfreeze support)
	if inFreeze, reason := s.freezeScheduleService.IsInFreezeWindowWithUnfreeze(k8sConfig.FreezeSchedules, pool.OneTimeUnfreezeUntil); inFreeze {
		log.Printf("[K8sDeployment] Pool %s is in freeze window: %s, skipping auto-scale", pool.PoolID, reason)
		return 0, false, nil
	}

	// Get current replicas
	_, currentReplicas, err := s.GetDeploymentReplicas(ctx, pool.PoolID)
	if err != nil {
		return 0, false, fmt.Errorf("failed to get current replicas: %w", err)
	}

	// Count active tasks (running only, not pending)
	activeTaskCount, err := s.CountPendingTasksForPool(ctx, pool.PoolID)
	if err != nil {
		return 0, false, fmt.Errorf("failed to count active tasks: %w", err)
	}

	// Check if there are "first pending tasks" waiting for agents
	// Only count the first pending task in each workspace (not blocked by other tasks)
	// This prevents counting tasks that are blocked by workspace locks or earlier tasks
	var firstPendingTaskCount int64

	// Use subquery to find workspaces where the first task is pending and not blocked
	err = s.db.WithContext(ctx).
		Table("workspace_tasks AS wt1").
		Joins("JOIN workspaces ON workspaces.workspace_id = wt1.workspace_id").
		Where("workspaces.current_pool_id = ?", pool.PoolID).
		Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
		Where("wt1.status = ?", models.TaskStatusPending).
		Where(`NOT EXISTS (
			SELECT 1 FROM workspace_tasks AS wt2 
			WHERE wt2.workspace_id = wt1.workspace_id 
			AND wt2.id < wt1.id 
			AND wt2.status IN ('pending', 'running', 'apply_pending')
		)`).
		Count(&firstPendingTaskCount).Error

	if err != nil {
		log.Printf("[K8sDeployment] Warning: failed to count first pending tasks: %v", err)
		firstPendingTaskCount = 0
	}

	// Determine desired replicas with gradual scale-down
	var desiredReplicas int32

	if activeTaskCount == 0 {
		// Check if we have "first pending tasks" (not blocked by other tasks in same workspace)
		if firstPendingTaskCount > 0 {
			// Has first pending tasks - but check if existing agents have capacity first
			// FIX: Check online agent capacity before scaling up
			onlineAgentCount, availableCapacity, err := s.getOnlineAgentCapacity(ctx, pool.PoolID)
			if err != nil {
				log.Printf("[K8sDeployment] Warning: failed to check agent capacity: %v, will scale conservatively", err)
				// On error, scale conservatively
				if currentReplicas == 0 {
					desiredReplicas = 1
				} else {
					desiredReplicas = currentReplicas
				}
			} else {
				log.Printf("[K8sDeployment] Pool %s capacity check: online_agents=%d, available_capacity=%d, first_pending_tasks=%d",
					pool.PoolID, onlineAgentCount, availableCapacity, firstPendingTaskCount)

				if onlineAgentCount == 0 {
					// Cold start - no agents online
					desiredReplicas = 1
					log.Printf("[K8sDeployment] Pool %s has %d first-pending tasks but 0 online agents (cold start), scaling to 1 pod",
						pool.PoolID, firstPendingTaskCount)
				} else if availableCapacity > 0 {
					// Has online agents with available capacity - don't scale up
					desiredReplicas = currentReplicas
					log.Printf("[K8sDeployment] Pool %s has %d first-pending tasks but %d online agents with %d available capacity, no scale-up needed",
						pool.PoolID, firstPendingTaskCount, onlineAgentCount, availableCapacity)
				} else {
					// Online agents are at full capacity - scale up gradually
					desiredReplicas = currentReplicas + 1
					if desiredReplicas > int32(k8sConfig.MaxReplicas) {
						desiredReplicas = int32(k8sConfig.MaxReplicas)
					}
					log.Printf("[K8sDeployment] Pool %s has %d first-pending tasks, %d online agents at full capacity, scaling to %d (gradual scale-up)",
						pool.PoolID, firstPendingTaskCount, onlineAgentCount, desiredReplicas)
				}
			}
			// Don't set idle time when we have first-pending tasks
		} else {
			// No active tasks - implement gradual scale-down
			idleTime, exists := s.poolIdleTimes[pool.PoolID]

			if !exists {
				// First time seeing this pool idle, record the time
				s.poolIdleTimes[pool.PoolID] = time.Now()
				log.Printf("[K8sDeployment] Pool %s became idle, starting idle timer", pool.PoolID)

				// Scale to 1 pod (keep warm for quick response)
				desiredReplicas = 1
			} else {
				// Pool has been idle for some time
				idleDuration := time.Since(idleTime)

				if idleDuration >= 1*time.Minute {
					// Idle for 1+ minutes, scale to 0 (save resources)
					desiredReplicas = 0
					log.Printf("[K8sDeployment] Pool %s idle for %v, scaling to 0 pods",
						pool.PoolID, idleDuration)
				} else {
					// Idle but less than 1 minute, keep 1 pod warm
					desiredReplicas = 1
					log.Printf("[K8sDeployment] Pool %s idle for %v, keeping 1 pod warm (will scale to 0 after 1min)",
						pool.PoolID, idleDuration)
				}
			}
		}
	} else {
		// Has active tasks - reset idle timer and scale to match workload
		delete(s.poolIdleTimes, pool.PoolID)

		desiredReplicas = int32(activeTaskCount)

		// Respect max_replicas constraint
		if desiredReplicas > int32(k8sConfig.MaxReplicas) {
			desiredReplicas = int32(k8sConfig.MaxReplicas)
		}

		// Ensure at least min_replicas
		if desiredReplicas < int32(k8sConfig.MinReplicas) {
			desiredReplicas = int32(k8sConfig.MinReplicas)
		}
	}

	// Only scale if there's a change
	if desiredReplicas == currentReplicas {
		return currentReplicas, false, nil
	}

	// FIX: Before scaling DOWN, verify no agents have running tasks
	// K8s scale-down is unordered and may terminate pods with active tasks
	if desiredReplicas < currentReplicas {
		hasRunningTasks, err := s.hasAnyAgentWithRunningTasks(ctx, pool.PoolID)
		if err != nil {
			log.Printf("[K8sDeployment] Warning: failed to check for running tasks before scale-down: %v", err)
			// Don't block scale-down on check failure, but log it
		} else if hasRunningTasks {
			log.Printf("[K8sDeployment] Skipping scale-down for pool %s: agents still have running tasks (current=%d, desired=%d)",
				pool.PoolID, currentReplicas, desiredReplicas)
			return currentReplicas, false, nil
		}
	}

	// Perform scaling
	if err := s.ScaleDeployment(ctx, pool.PoolID, desiredReplicas); err != nil {
		return 0, false, fmt.Errorf("failed to scale deployment: %w", err)
	}

	log.Printf("[K8sDeployment] Auto-scaled pool %s from %d to %d replicas (active tasks: %d, running only)",
		pool.PoolID, currentReplicas, desiredReplicas, activeTaskCount)

	return desiredReplicas, true, nil
}

// hasAnyAgentWithRunningTasks checks if any agent in the pool has running tasks
// This prevents scale-down from terminating agents that are actively executing tasks
func (s *K8sDeploymentService) hasAnyAgentWithRunningTasks(ctx context.Context, poolID string) (bool, error) {
	// Check if there are any running tasks assigned to agents in this pool
	var count int64
	err := s.db.WithContext(ctx).
		Model(&models.WorkspaceTask{}).
		Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
		Where("workspaces.current_pool_id = ?", poolID).
		Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
		Where("workspace_tasks.agent_id IS NOT NULL AND workspace_tasks.agent_id != ''").
		Where("workspace_tasks.status IN (?)", []models.TaskStatus{
			models.TaskStatusRunning,
			models.TaskStatusApplyPending,
		}).
		Count(&count).Error

	if err != nil {
		return false, fmt.Errorf("failed to check for running tasks: %w", err)
	}

	if count > 0 {
		log.Printf("[K8sDeployment] Pool %s has %d tasks with assigned agents in running/apply_pending status", poolID, count)
	}

	return count > 0, nil
}

// StartAutoScaler starts a goroutine that periodically checks and scales deployments
func (s *K8sDeploymentService) StartAutoScaler(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[K8sDeployment] Starting auto-scaler with interval: %v", interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[K8sDeployment] Auto-scaler stopped")
			return
		case <-ticker.C:
			s.runAutoScalerCycle(ctx)
		}
	}
}

// runAutoScalerCycle runs one cycle of auto-scaling for all K8s pools
// Now uses Pod-based management with slot reconciliation
func (s *K8sDeploymentService) runAutoScalerCycle(ctx context.Context) {
	// Get all K8s pools
	var pools []models.AgentPool
	err := s.db.WithContext(ctx).
		Where("pool_type = ?", models.AgentPoolTypeK8s).
		Find(&pools).Error

	if err != nil {
		log.Printf("[K8sPodService] Error fetching K8s pools: %v", err)
		return
	}

	for _, pool := range pools {
		// 1. Reconcile Pods (sync state from K8s and database)
		if err := s.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
			log.Printf("[K8sPodService] Error reconciling pods for pool %s: %v", pool.PoolID, err)
		}

		// 2. Check and restart unhealthy pods (pods without registered agents)
		if err := s.checkAndRestartUnhealthyPods(ctx, &pool); err != nil {
			log.Printf("[K8sPodService] Error checking unhealthy pods for pool %s: %v", pool.PoolID, err)
		}

		// 3. Check and rotate secret if needed
		if err := s.checkAndRotateSecret(ctx, &pool); err != nil {
			log.Printf("[K8sPodService] Error checking secret rotation for pool %s: %v", pool.PoolID, err)
		}

		// 4. Auto-scale based on slot utilization
		_, scaled, err := s.AutoScalePods(ctx, &pool)
		if err != nil {
			log.Printf("[K8sPodService] Error auto-scaling pool %s: %v", pool.PoolID, err)
			continue
		}

		if scaled {
			log.Printf("[K8sPodService] Successfully scaled pool %s", pool.PoolID)
		}
	}
}

// checkAndRestartUnhealthyPods checks for pods that exist in K8s but have no registered agent
// and restarts them with exponential backoff to avoid pod thrashing
//
// IMPORTANT: Agent ID format is "agent-{poolID}-{timestamp_nano}" (e.g., agent-pool-xxx-1764763303124306000)
// Pod name format is "iac-agent-{poolID}-{timestamp_seconds}" (e.g., iac-agent-pool-xxx-1764763302)
// These do NOT match, so we cannot use pod name to find agent.
//
// Unhealthy pod detection (SAFE approach):
// - Pod exists in K8s (Running phase)
// - Pod has been running for more than 2 minutes (grace period for agent startup)
// - Check if there are ANY online agents for this pool (by last_ping_at within 2 minutes)
// - Check if there are ANY running tasks for this pool
// - Only restart if: no online agents AND no running tasks AND pod count matches expected
//
// Exponential backoff:
// - First restart: immediate (after 2 min grace period)
// - Second restart: wait 1 minute
// - Third restart: wait 2 minutes
// - Fourth restart: wait 4 minutes
// - Max backoff: 10 minutes
// - Max restarts: 5 (then give up and log error)
func (s *K8sDeploymentService) checkAndRestartUnhealthyPods(ctx context.Context, pool *models.AgentPool) error {
	namespace := "terraform"

	// List all pods for this pool from K8s
	labelSelector := fmt.Sprintf("app=iac-platform,component=agent,pool-id=%s", pool.PoolID)
	podList, err := s.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	// Get all registered agents for this pool
	var agents []models.Agent
	if err := s.db.WithContext(ctx).Where("pool_id = ?", pool.PoolID).Find(&agents).Error; err != nil {
		return fmt.Errorf("failed to query agents: %w", err)
	}

	// Count online agents (last_ping_at within 2 minutes)
	now := time.Now()
	onlineAgentCount := 0
	for _, agent := range agents {
		if agent.LastPingAt != nil && now.Sub(*agent.LastPingAt) < 2*time.Minute {
			onlineAgentCount++
		}
	}

	// Count running pods (in Running phase)
	runningPodCount := 0
	for _, k8sPod := range podList.Items {
		if k8sPod.Status.Phase == corev1.PodRunning {
			runningPodCount++
		}
	}

	// Check if there are any running tasks for this pool
	var runningTaskCount int64
	err = s.db.WithContext(ctx).
		Model(&models.WorkspaceTask{}).
		Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
		Where("workspaces.current_pool_id = ?", pool.PoolID).
		Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
		Where("workspace_tasks.status IN (?)", []models.TaskStatus{
			models.TaskStatusRunning,
			models.TaskStatusApplyPending,
		}).
		Count(&runningTaskCount).Error
	if err != nil {
		log.Printf("[K8sPodService] Warning: failed to count running tasks: %v", err)
		// On error, don't restart pods to be safe
		return nil
	}

	// SAFETY CHECK: If there are running tasks, NEVER restart any pods
	// This prevents killing pods that are actively executing tasks
	if runningTaskCount > 0 {
		log.Printf("[K8sPodService] Pool %s has %d running tasks, skipping unhealthy pod check",
			pool.PoolID, runningTaskCount)
		return nil
	}

	// SAFETY CHECK: If online agents >= running pods, all pods have agents
	// This means pods are healthy, just agent ID doesn't match pod name
	if onlineAgentCount >= runningPodCount {
		log.Printf("[K8sPodService] Pool %s has %d online agents for %d running pods, all healthy",
			pool.PoolID, onlineAgentCount, runningPodCount)
		// Clear all restart info since pods are healthy
		s.podRestartMu.Lock()
		for podName := range s.podRestartInfo {
			delete(s.podRestartInfo, podName)
		}
		s.podRestartMu.Unlock()
		return nil
	}

	// At this point: no running tasks AND online agents < running pods
	// Some pods may be unhealthy (no agent registered)
	unhealthyPodCount := runningPodCount - onlineAgentCount
	log.Printf("[K8sPodService] Pool %s may have %d unhealthy pods (running_pods=%d, online_agents=%d)",
		pool.PoolID, unhealthyPodCount, runningPodCount, onlineAgentCount)

	gracePeriod := 2 * time.Minute // Wait 2 minutes for agent to register after pod starts

	// Find pods that are old enough to be considered unhealthy
	var podsToRestart []string
	for _, k8sPod := range podList.Items {
		podName := k8sPod.Name

		// Skip pods that are not in Running phase
		if k8sPod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Skip pods that haven't been running long enough (grace period)
		podAge := now.Sub(k8sPod.CreationTimestamp.Time)
		if podAge < gracePeriod {
			continue
		}

		// This pod is old enough and might be unhealthy
		podsToRestart = append(podsToRestart, podName)
	}

	// Only restart up to unhealthyPodCount pods (to avoid restarting healthy ones)
	if len(podsToRestart) > unhealthyPodCount {
		podsToRestart = podsToRestart[:unhealthyPodCount]
	}

	for _, podName := range podsToRestart {
		// Check restart info and apply exponential backoff
		s.podRestartMu.Lock()
		info, exists := s.podRestartInfo[podName]
		if !exists {
			// First time detecting this pod as unhealthy
			info = &PodRestartInfo{
				RestartCount:     0,
				FirstUnhealthyAt: now,
			}
			s.podRestartInfo[podName] = info
		}

		// Calculate backoff duration based on restart count
		// Backoff: 0, 1min, 2min, 4min, 8min (capped at 10min)
		var backoffDuration time.Duration
		if info.RestartCount == 0 {
			backoffDuration = 0 // First restart is immediate (after grace period)
		} else {
			backoffDuration = time.Duration(1<<(info.RestartCount-1)) * time.Minute
			if backoffDuration > 10*time.Minute {
				backoffDuration = 10 * time.Minute
			}
		}

		// Check if we should restart now
		timeSinceLastRestart := now.Sub(info.LastRestartTime)
		if info.RestartCount > 0 && timeSinceLastRestart < backoffDuration {
			// Still in backoff period
			log.Printf("[K8sPodService] Pod %s is unhealthy but in backoff period (restart #%d, wait %v, elapsed %v)",
				podName, info.RestartCount, backoffDuration, timeSinceLastRestart)
			s.podRestartMu.Unlock()
			continue
		}

		// Check max restart limit
		maxRestarts := 5
		if info.RestartCount >= maxRestarts {
			log.Printf("[K8sPodService] ERROR: Pod %s has been restarted %d times but agent still not registering. "+
				"Manual intervention required. First unhealthy at: %v",
				podName, info.RestartCount, info.FirstUnhealthyAt)
			s.podRestartMu.Unlock()
			continue
		}

		// Update restart info
		info.RestartCount++
		info.LastRestartTime = now
		s.podRestartMu.Unlock()

		// Restart the pod by deleting it (K8s will not recreate it since RestartPolicy is Never)
		// We need to delete and recreate
		log.Printf("[K8sPodService] Restarting unhealthy pod %s (restart #%d, backoff was %v)",
			podName, info.RestartCount, backoffDuration)

		// Delete the pod
		propagationPolicy := metav1.DeletePropagationBackground
		if err := s.clientset.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		}); err != nil && !errors.IsNotFound(err) {
			log.Printf("[K8sPodService] Failed to delete unhealthy pod %s: %v", podName, err)
			continue
		}

		// Remove from pod manager
		s.podManager.mu.Lock()
		delete(s.podManager.pods, podName)
		s.podManager.mu.Unlock()

		log.Printf("[K8sPodService] Deleted unhealthy pod %s, will be recreated by auto-scaler", podName)
	}

	// Clean up restart info for pods that no longer exist
	s.podRestartMu.Lock()
	existingPods := make(map[string]bool)
	for _, k8sPod := range podList.Items {
		existingPods[k8sPod.Name] = true
	}
	for podName := range s.podRestartInfo {
		if !existingPods[podName] {
			delete(s.podRestartInfo, podName)
		}
	}
	s.podRestartMu.Unlock()

	return nil
}

// checkAndRotateSecret checks if secret needs rotation and rotates it if deployment is scaled to 0
func (s *K8sDeploymentService) checkAndRotateSecret(ctx context.Context, pool *models.AgentPool) error {
	secretName := fmt.Sprintf("iac-agent-token-%s", pool.PoolID)
	namespace := "terraform"

	// Get secret
	secret, err := s.clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Secret doesn't exist, will be created when deployment is ensured
			return nil
		}
		return fmt.Errorf("failed to get secret: %w", err)
	}

	// Check age
	age := time.Since(secret.CreationTimestamp.Time)
	if age <= 30*24*time.Hour {
		// Secret is still fresh
		return nil
	}

	log.Printf("[K8sDeployment] Secret %s is %v old, checking if rotation is possible", secretName, age)

	// Check if deployment has 0 replicas
	_, currentReplicas, err := s.GetDeploymentReplicas(ctx, pool.PoolID)
	if err != nil {
		return fmt.Errorf("failed to check replicas: %w", err)
	}

	if currentReplicas > 0 {
		log.Printf("[K8sDeployment] Cannot rotate secret %s: deployment has %d replicas (must be 0)", secretName, currentReplicas)
		return nil
	}

	// Safe to rotate
	log.Printf("[K8sDeployment] Rotating secret %s (deployment has 0 replicas)", secretName)
	return s.rotateSecret(ctx, pool, secretName, namespace)
}

// RebuildIdlePods rebuilds all idle pods with the latest configuration
// This is called after K8s config updates to apply new settings to idle pods
// Only pods where ALL slots are idle will be deleted and recreated
func (s *K8sDeploymentService) RebuildIdlePods(ctx context.Context, pool *models.AgentPool) error {
	if pool.PoolType != models.AgentPoolTypeK8s {
		return fmt.Errorf("pool %s is not a K8s pool", pool.PoolID)
	}

	log.Printf("[K8sPodService] Rebuilding idle pods for pool %s after config update", pool.PoolID)

	// 1. Sync pods from K8s to ensure we have latest state
	if err := s.podManager.SyncPodsFromK8s(ctx, pool.PoolID); err != nil {
		log.Printf("[K8sPodService] Warning: failed to sync pods: %v", err)
	}

	// 2. Reconcile pods to sync task status to slots
	if err := s.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
		log.Printf("[K8sPodService] Warning: failed to reconcile pods: %v", err)
	}

	// 3. Find all idle pods (all slots are idle)
	idlePods := s.podManager.FindIdlePods(pool.PoolID)
	if len(idlePods) == 0 {
		log.Printf("[K8sPodService] No idle pods found for pool %s, nothing to rebuild", pool.PoolID)
		return nil
	}

	log.Printf("[K8sPodService] Found %d idle pods to rebuild for pool %s", len(idlePods), pool.PoolID)

	// 4. Parse K8s config for recreation
	if pool.K8sConfig == nil {
		return fmt.Errorf("pool %s does not have K8s configuration", pool.PoolID)
	}

	var k8sConfig models.K8sJobTemplateConfig
	if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err != nil {
		return fmt.Errorf("failed to parse K8s config: %w", err)
	}

	// 5. Ensure secret exists
	secretName, err := s.EnsureSecretForPool(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to ensure secret: %w", err)
	}

	// 6. Delete idle pods
	deletedCount := 0
	for _, pod := range idlePods {
		if err := s.podManager.DeletePod(ctx, pod.PodName); err != nil {
			log.Printf("[K8sPodService] Warning: failed to delete idle pod %s: %v", pod.PodName, err)
		} else {
			deletedCount++
			log.Printf("[K8sPodService] Deleted idle pod %s for rebuild (%d/%d)", pod.PodName, deletedCount, len(idlePods))
		}
	}

	if deletedCount == 0 {
		log.Printf("[K8sPodService] Warning: failed to delete any idle pods, skipping recreation")
		return nil
	}

	// 7. Wait briefly for pods to terminate
	time.Sleep(2 * time.Second)

	// 8. Recreate pods with new configuration
	for i := 0; i < deletedCount; i++ {
		_, err := s.podManager.CreatePod(ctx, pool.PoolID, &k8sConfig, secretName)
		if err != nil {
			log.Printf("[K8sPodService] Warning: failed to create replacement pod %d/%d: %v", i+1, deletedCount, err)
		} else {
			log.Printf("[K8sPodService] Created replacement pod with new config (%d/%d)", i+1, deletedCount)
		}
	}

	log.Printf("[K8sPodService] Successfully rebuilt %d idle pods for pool %s with updated configuration", deletedCount, pool.PoolID)
	return nil
}

// ForceRotateToken forces token rotation, updates secret, and rebuilds all Pods
// This is called from the API when user manually triggers rotation or revoke
func (s *K8sDeploymentService) ForceRotateToken(ctx context.Context, pool *models.AgentPool, rotatedBy string) error {
	secretName := fmt.Sprintf("iac-agent-token-%s", pool.PoolID)
	namespace := "terraform"

	log.Printf("[K8sPodService] Force rotating token for pool %s by user %s", pool.PoolID, rotatedBy)

	// 1. Delete all Pods (both idle and busy) to force complete rebuild
	// Sync Pods first to ensure we have latest state
	if err := s.podManager.SyncPodsFromK8s(ctx, pool.PoolID); err != nil {
		log.Printf("[K8sPodService] Warning: failed to sync pods: %v", err)
	}

	allPods := s.podManager.ListPods(pool.PoolID)
	if len(allPods) > 0 {
		log.Printf("[K8sPodService] Deleting %d pods before token rotation", len(allPods))
		for _, pod := range allPods {
			if err := s.podManager.DeletePod(ctx, pod.PodName); err != nil {
				log.Printf("[K8sPodService] Warning: failed to delete pod %s: %v", pod.PodName, err)
			} else {
				log.Printf("[K8sPodService] Deleted pod %s for token rotation", pod.PodName)
			}
		}

		// Wait for pods to terminate (max 30 seconds)
		for i := 0; i < 30; i++ {
			time.Sleep(1 * time.Second)
			if err := s.podManager.SyncPodsFromK8s(ctx, pool.PoolID); err == nil {
				if s.podManager.GetPodCount(pool.PoolID) == 0 {
					log.Printf("[K8sPodService] All pods terminated")
					break
				}
			}
			if i == 29 {
				log.Printf("[K8sPodService] Warning: timeout waiting for pods to terminate, proceeding anyway")
			}
		}
	}

	// 2. Rotate secret (revoke old token + generate new token + update K8s secret)
	if err := s.rotateSecret(ctx, pool, secretName, namespace); err != nil {
		return fmt.Errorf("failed to rotate secret: %w", err)
	}

	// 3. Rebuild Pods with new token by calling EnsurePodsForPool
	log.Printf("[K8sPodService] Rebuilding pods with new token for pool %s", pool.PoolID)
	if err := s.EnsurePodsForPool(ctx, pool); err != nil {
		return fmt.Errorf("failed to rebuild pods: %w", err)
	}

	log.Printf("[K8sPodService] Successfully force rotated token for pool %s", pool.PoolID)
	return nil
}

// restartDeployment triggers a rollout restart by updating an annotation
func (s *K8sDeploymentService) restartDeployment(ctx context.Context, poolID string) error {
	deploymentName := fmt.Sprintf("iac-agent-%s", poolID)
	namespace := "terraform"

	deployment, err := s.clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Add/update restart annotation to trigger rollout
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	_, err = s.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	log.Printf("[K8sDeployment] Triggered rollout restart for deployment %s", deploymentName)
	return nil
}

// getOnlineAgentCapacity checks online agents and calculates available capacity
// Returns: (online agent count, available capacity slots, error)
//
// Capacity calculation:
// - Each agent can handle: 3 plan tasks + 1 plan_and_apply task
// - Available capacity = total capacity - currently running tasks
func (s *K8sDeploymentService) getOnlineAgentCapacity(ctx context.Context, poolID string) (int, int, error) {
	// Get all online agents in this pool
	var agents []models.Agent
	err := s.db.WithContext(ctx).
		Where("pool_id = ?", poolID).
		Find(&agents).Error

	if err != nil {
		return 0, 0, fmt.Errorf("failed to query agents: %w", err)
	}

	// Filter online agents (last ping within 2 minutes)
	onlineAgents := make([]models.Agent, 0)
	for _, agent := range agents {
		if agent.IsOnline() {
			onlineAgents = append(onlineAgents, agent)
		}
	}

	onlineCount := len(onlineAgents)
	if onlineCount == 0 {
		return 0, 0, nil
	}

	// Calculate total capacity: each agent can handle 3 plan tasks + 1 plan_and_apply task
	// For simplicity, we use a combined capacity of 3 slots per agent
	totalCapacity := onlineCount * 3

	// Count currently running tasks assigned to these agents
	agentIDs := make([]string, len(onlineAgents))
	for i, agent := range onlineAgents {
		agentIDs[i] = agent.AgentID
	}

	var runningTaskCount int64
	err = s.db.WithContext(ctx).
		Model(&models.WorkspaceTask{}).
		Where("agent_id IN (?)", agentIDs).
		Where("status IN (?)", []models.TaskStatus{
			models.TaskStatusRunning,
			models.TaskStatusApplyPending,
		}).
		Count(&runningTaskCount).Error

	if err != nil {
		return onlineCount, 0, fmt.Errorf("failed to count running tasks: %w", err)
	}

	// Calculate available capacity
	availableCapacity := totalCapacity - int(runningTaskCount)
	if availableCapacity < 0 {
		availableCapacity = 0
	}

	log.Printf("[K8sDeployment] Pool %s capacity: online_agents=%d, total_capacity=%d, running_tasks=%d, available=%d",
		poolID, onlineCount, totalCapacity, runningTaskCount, availableCapacity)

	return onlineCount, availableCapacity, nil
}

// generateTokenName generates a token name with format: tn-{16位随机a-z0-9}
func generateTokenName() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 16

	b := make([]byte, length)
	for i := range b {
		n, err := rand.Read(b[i : i+1])
		if err != nil || n != 1 {
			return "", fmt.Errorf("failed to generate random bytes: %w", err)
		}
		b[i] = charset[int(b[i])%len(charset)]
	}

	return fmt.Sprintf("tn-%s", string(b)), nil
}
