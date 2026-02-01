package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"iac-platform/internal/models"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"gorm.io/gorm"
)

// K8sJobService handles K8s Job creation and management
type K8sJobService struct {
	db                    *gorm.DB
	clientset             *kubernetes.Clientset
	freezeScheduleService *FreezeScheduleService
}

// NewK8sJobService creates a new K8s Job service
func NewK8sJobService(db *gorm.DB) (*K8sJobService, error) {
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

	return &K8sJobService{
		db:                    db,
		clientset:             clientset,
		freezeScheduleService: NewFreezeScheduleService(),
	}, nil
}

// CreateJobForTask creates a K8s Job for a workspace task
func (s *K8sJobService) CreateJobForTask(ctx context.Context, task *models.WorkspaceTask, pool *models.AgentPool) error {
	// 1. Check if pool has K8s config
	if pool.K8sConfig == nil {
		return fmt.Errorf("pool %s does not have K8s configuration", pool.PoolID)
	}

	var k8sConfig models.K8sJobTemplateConfig
	if pool.K8sConfig != nil {
		if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err != nil {
			return fmt.Errorf("failed to parse K8s config: %w", err)
		}
	} else {
		return fmt.Errorf("pool %s has no K8s configuration", pool.PoolID)
	}

	// 2. Check freeze schedule
	if inFreeze, reason := s.freezeScheduleService.IsInFreezeWindow(k8sConfig.FreezeSchedules); inFreeze {
		log.Printf("[K8sJob] Pool %s is in freeze window: %s", pool.PoolID, reason)
		return fmt.Errorf("pool is in freeze window: %s", reason)
	}

	// 3. Check idempotency - if job already exists for this task
	jobName := fmt.Sprintf("%s-%d", task.WorkspaceID, task.ID)
	namespace := "terraform" // Default namespace
	if k8sConfig.Resources != nil && k8sConfig.Resources.Limits != nil {
		if ns, ok := k8sConfig.Resources.Limits["namespace"]; ok {
			namespace = ns
		}
	}

	// Check if token already exists
	var existingToken models.PoolToken
	err := s.db.Where("k8s_job_name = ? AND pool_id = ?", jobName, pool.PoolID).First(&existingToken).Error
	if err == nil {
		// Token exists, check if job exists in K8s
		_, err := s.clientset.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
		if err == nil {
			log.Printf("[K8sJob] Job %s already exists, skipping creation", jobName)
			return nil // Job already exists, idempotent
		}
		// Job doesn't exist but token does - clean up token and recreate
		log.Printf("[K8sJob] Token exists but job doesn't, cleaning up token")
		s.db.Delete(&existingToken)
	}

	// 4. Generate temporary token
	token, tokenHash, err := s.generateToken()
	if err != nil {
		return fmt.Errorf("failed to generate token: %w", err)
	}

	// 5. Calculate expiration (2-4 hours)
	expiresAt := time.Now().Add(3 * time.Hour)

	// 6. Create pool token record
	poolToken := &models.PoolToken{
		TokenHash:    tokenHash,
		TokenName:    fmt.Sprintf("k8s-job-%s-%d", task.WorkspaceID, task.ID),
		TokenType:    models.PoolTokenTypeK8sTemporary,
		PoolID:       pool.PoolID,
		IsActive:     true,
		ExpiresAt:    &expiresAt,
		K8sJobName:   &jobName,
		K8sNamespace: namespace,
	}

	if err := s.db.Create(poolToken).Error; err != nil {
		return fmt.Errorf("failed to create pool token: %w", err)
	}

	// 7. Create K8s Job
	job := s.buildJob(jobName, namespace, task, &k8sConfig, token, pool.PoolID)

	_, err = s.clientset.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		// Clean up token if job creation fails
		s.db.Delete(poolToken)
		if errors.IsAlreadyExists(err) {
			log.Printf("[K8sJob] Job %s already exists (race condition)", jobName)
			return nil // Idempotent
		}
		return fmt.Errorf("failed to create K8s job: %w", err)
	}

	log.Printf("[K8sJob] Successfully created job %s in namespace %s", jobName, namespace)

	// 8. Update task with K8s info
	task.K8sPodName = jobName
	task.K8sNamespace = namespace
	if err := s.db.Save(task).Error; err != nil {
		log.Printf("[K8sJob] Warning: failed to update task with K8s info: %v", err)
	}

	return nil
}

// buildJob constructs a K8s Job object
func (s *K8sJobService) buildJob(jobName, namespace string, task *models.WorkspaceTask, config *models.K8sJobTemplateConfig, token, poolID string) *batchv1.Job {
	// Default values
	backoffLimit := int32(3)
	if config.BackoffLimit != nil {
		backoffLimit = int32(*config.BackoffLimit)
	}

	ttlSeconds := int32(600)
	if config.TTLSecondsAfter != nil {
		ttlSeconds = int32(*config.TTLSecondsAfter)
	}

	activeDeadline := int64(7200) // 2 hours

	restartPolicy := corev1.RestartPolicyNever
	if config.RestartPolicy != "" {
		restartPolicy = corev1.RestartPolicy(config.RestartPolicy)
	}

	imagePullPolicy := corev1.PullIfNotPresent
	if config.ImagePullPolicy != "" {
		imagePullPolicy = corev1.PullPolicy(config.ImagePullPolicy)
	}

	// Build environment variables
	apiEndpoint := os.Getenv("API_ENDPOINT")
	if apiEndpoint == "" {
		// 如果环境变量未设置，使用默认值
		apiEndpoint = "http://localhost:8080"
		log.Printf("[K8sJob] API_ENDPOINT not set, using default: %s", apiEndpoint)
	}

	envVars := []corev1.EnvVar{
		{Name: "TASK_ID", Value: fmt.Sprintf("%d", task.ID)},
		{Name: "WORKSPACE_ID", Value: task.WorkspaceID},
		{Name: "AGENT_TOKEN", Value: token},
		{Name: "API_ENDPOINT", Value: apiEndpoint},
		{Name: "POOL_ID", Value: poolID},
	}

	// Add custom env vars from config
	for key, value := range config.Env {
		envVars = append(envVars, corev1.EnvVar{Name: key, Value: value})
	}

	// Build resource requirements
	resources := corev1.ResourceRequirements{}
	if config.Resources != nil {
		if config.Resources.Requests != nil {
			resources.Requests = make(corev1.ResourceList)
			// For now, skip resource parsing - will be added in future optimization
			// TODO: Implement proper resource.MustParse for CPU and memory
		}
		if config.Resources.Limits != nil {
			resources.Limits = make(corev1.ResourceList)
			// For now, skip resource parsing - will be added in future optimization
			// TODO: Implement proper resource.MustParse for CPU and memory
		}
	}

	// Build container
	container := corev1.Container{
		Name:            "terraform-agent",
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

	// Build Job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":          "iac-platform",
				"component":    "terraform-agent",
				"workspace-id": task.WorkspaceID,
				"task-id":      fmt.Sprintf("%d", task.ID),
				"pool-id":      poolID,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			ActiveDeadlineSeconds:   &activeDeadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":          "iac-platform",
						"component":    "terraform-agent",
						"workspace-id": task.WorkspaceID,
						"task-id":      fmt.Sprintf("%d", task.ID),
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: restartPolicy,
					Containers:    []corev1.Container{container},
				},
			},
		},
	}

	return job
}

// generateToken generates a random token and its hash
func (s *K8sJobService) generateToken() (string, string, error) {
	// Generate 32 bytes of random data
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}

	// Convert to hex string (64 characters)
	token := hex.EncodeToString(tokenBytes)

	// Generate SHA-256 hash
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	return token, tokenHash, nil
}

// parseQuantity is a helper to parse resource quantity strings
func parseQuantity(value string) string {
	// Return the value as-is for now
	// K8s will parse it when creating the Job
	return value
}

// GetJobStatus retrieves the status of a K8s Job
func (s *K8sJobService) GetJobStatus(ctx context.Context, jobName, namespace string) (*batchv1.JobStatus, error) {
	job, err := s.clientset.BatchV1().Jobs(namespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return &job.Status, nil
}

// DeleteJob deletes a K8s Job
func (s *K8sJobService) DeleteJob(ctx context.Context, jobName, namespace string) error {
	propagationPolicy := metav1.DeletePropagationBackground
	return s.clientset.BatchV1().Jobs(namespace).Delete(ctx, jobName, metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
}
