package leaderelection

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	k8sleaderelection "k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// Config holds the configuration for leader election.
type Config struct {
	// LeaseName is the name of the Lease resource used for leader election.
	LeaseName string
	// LeaseNamespace is the Kubernetes namespace where the Lease resource lives.
	LeaseNamespace string
	// Identity is the unique identity of this participant in the election.
	Identity string
	// LeaseDuration is the duration a non-leader waits before attempting to acquire the lease.
	LeaseDuration time.Duration
	// RenewDeadline is the duration the leader retries refreshing leadership before giving up.
	RenewDeadline time.Duration
	// RetryPeriod is how long participants wait between election attempts.
	RetryPeriod time.Duration
}

// DefaultConfig returns a Config populated from environment variables and sensible defaults.
// POD_NAME is used as Identity (falls back to hostname).
// POD_NAMESPACE is used as LeaseNamespace (falls back to "default").
// LeaseName defaults to "iac-platform-leader".
func DefaultConfig() Config {
	identity := os.Getenv("POD_NAME")
	if identity == "" {
		h, err := os.Hostname()
		if err != nil {
			identity = "unknown"
		} else {
			identity = h
		}
	}

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}

	return Config{
		LeaseName:      "iac-platform-leader",
		LeaseNamespace: namespace,
		Identity:       identity,
		LeaseDuration:  15 * time.Second,
		RenewDeadline:  10 * time.Second,
		RetryPeriod:    2 * time.Second,
	}
}

// LeaderCallbacks defines the callbacks invoked during leader election lifecycle events.
type LeaderCallbacks struct {
	// OnStartedLeading is called when this instance becomes the leader.
	// The provided context is cancelled when leadership is lost.
	OnStartedLeading func(ctx context.Context)
	// OnStoppedLeading is called when this instance stops being the leader.
	OnStoppedLeading func()
	// OnNewLeader is called when a new leader is observed (including the first leader).
	OnNewLeader func(identity string)
}

// buildKubeConfig attempts to build a Kubernetes rest.Config.
// It first tries in-cluster config, then falls back to KUBECONFIG env var.
func buildKubeConfig() (*rest.Config, error) {
	// Try in-cluster first.
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}

	// Fall back to KUBECONFIG env var.
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from KUBECONFIG=%s: %w", kubeconfig, err)
		}
		return cfg, nil
	}

	return nil, fmt.Errorf("unable to build kubernetes config: not in cluster and KUBECONFIG not set")
}

// Run starts the leader election loop using a Kubernetes Lease resource.
// It blocks until ctx is cancelled or leadership is lost.
func Run(ctx context.Context, cfg Config, callbacks LeaderCallbacks) error {
	restCfg, err := buildKubeConfig()
	if err != nil {
		return fmt.Errorf("leaderelection: %w", err)
	}

	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("leaderelection: failed to create kubernetes client: %w", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.LeaseName,
			Namespace: cfg.LeaseNamespace,
		},
		Client:     client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: cfg.Identity,
		},
	}

	// Wrap callbacks so nil functions are safe to call.
	onStarted := callbacks.OnStartedLeading
	if onStarted == nil {
		onStarted = func(ctx context.Context) {}
	}
	onStopped := callbacks.OnStoppedLeading
	if onStopped == nil {
		onStopped = func() {}
	}

	lec := k8sleaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   cfg.LeaseDuration,
		RenewDeadline:   cfg.RenewDeadline,
		RetryPeriod:     cfg.RetryPeriod,
		ReleaseOnCancel: true,
		Name:            cfg.LeaseName,
		Callbacks: k8sleaderelection.LeaderCallbacks{
			OnStartedLeading: onStarted,
			OnStoppedLeading: onStopped,
			OnNewLeader:      callbacks.OnNewLeader,
		},
	}

	le, err := k8sleaderelection.NewLeaderElector(lec)
	if err != nil {
		return fmt.Errorf("leaderelection: failed to create elector: %w", err)
	}

	// le.Run blocks until ctx is done or leadership is lost.
	le.Run(ctx)
	return nil
}

// RunWithFallback attempts to run K8s-based leader election. If no Kubernetes
// cluster is available (e.g. local development), it falls back to running as
// a standalone leader immediately.
func RunWithFallback(ctx context.Context, callbacks LeaderCallbacks) {
	cfg := DefaultConfig()

	_, err := buildKubeConfig()
	if err != nil {
		// No K8s cluster available -- run as standalone leader.
		log.Printf("leaderelection: kubernetes unavailable (%v), running as standalone leader", err)

		if callbacks.OnNewLeader != nil {
			callbacks.OnNewLeader(cfg.Identity)
		}
		if callbacks.OnStartedLeading != nil {
			callbacks.OnStartedLeading(ctx)
		}

		// Block until context is cancelled.
		<-ctx.Done()

		if callbacks.OnStoppedLeading != nil {
			callbacks.OnStoppedLeading()
		}
		return
	}

	if err := Run(ctx, cfg, callbacks); err != nil {
		log.Printf("leaderelection: election failed (%v), falling back to standalone leader", err)

		if callbacks.OnNewLeader != nil {
			callbacks.OnNewLeader(cfg.Identity)
		}
		if callbacks.OnStartedLeading != nil {
			callbacks.OnStartedLeading(ctx)
		}

		<-ctx.Done()

		if callbacks.OnStoppedLeading != nil {
			callbacks.OnStoppedLeading()
		}
	}
}
