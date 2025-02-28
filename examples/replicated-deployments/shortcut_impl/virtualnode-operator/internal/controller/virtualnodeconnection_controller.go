package controller

import (
	"context"
	"fmt"
	networkingv1alpha1 "github.com/nates110/my-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"os/exec"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

// VirtualNodeConnectionReconciler reconciles a VirtualNodeConnection object
type VirtualNodeConnectionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=networking.liqo.io,resources=virtualnodeconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.liqo.io,resources=virtualnodeconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=networking.liqo.io,resources=virtualnodeconnections/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VirtualNodeConnection object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile

const finalizerName = "virtualnodeconnection.finalizers.networking.liqo.io"

// Reconcile gestisce la creazione e la cancellazione delle VirtualNodeConnection
func (r *VirtualNodeConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling VirtualNodeConnection", "namespace", req.Namespace, "name", req.Name)

	var connection networkingv1alpha1.VirtualNodeConnection
	if err := r.Get(ctx, req.NamespacedName, &connection); err != nil {
		// Se la risorsa non viene trovata, non è necessario riconciliarla ulteriormente.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Se l'oggetto è in fase di eliminazione, esegui la disconnessione
	if !connection.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.Info("VirtualNodeConnection è in fase di eliminazione, avvio disconnessione", "name", req.Name)
		if err := r.disconnectLiqoctl(ctx, &connection); err != nil {
			logger.Error(err, "Errore durante la disconnessione")
			return ctrl.Result{}, err
		}

		// Rimuove il finalizer per permettere l'eliminazione dell'oggetto
		controllerutil.RemoveFinalizer(&connection, finalizerName)
		if err := r.Update(ctx, &connection); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("Finalizer rimosso, VirtualNodeConnection può essere eliminata", "name", req.Name)
		return ctrl.Result{}, nil
	}

	// Aggiunge il finalizer se non è già presente
	if !controllerutil.ContainsFinalizer(&connection, finalizerName) {
		logger.Info("Aggiungo il finalizer", "name", req.Name)
		controllerutil.AddFinalizer(&connection, finalizerName)
		if err := r.Update(ctx, &connection); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Se i nodi sono già connessi, non eseguire ulteriori azioni
	if connection.Status.IsConnected {
		logger.Info("Nodi già connessi", "nodeA", connection.Spec.VirtualNodeA, "nodeB", connection.Spec.VirtualNodeB)
		return ctrl.Result{}, nil
	}

	// Inizializza lo stato se non è stato impostato
	if connection.Status.Phase == "" {
		connection.Status = networkingv1alpha1.VirtualNodeConnectionStatus{
			IsConnected:  false,
			LastUpdated:  time.Now().Format(time.RFC3339),
			Phase:        "Pending",
			ErrorMessage: "",
		}
		if err := r.Status().Update(ctx, &connection); err != nil {
			logger.Error(err, "Errore nell'inizializzazione dello stato")
			return ctrl.Result{}, err
		}
	}

	logger.Info("Avvio connessione", "nodeA", connection.Spec.VirtualNodeA, "nodeB", connection.Spec.VirtualNodeB)
	if err := r.updateStatus(ctx, &connection, "Connecting", ""); err != nil {
		return ctrl.Result{}, err
	}

	// Esegue il comando liqoctl connect
	output, err := r.executeLiqoctlConnect(ctx, &connection)
	if err != nil {
		logger.Error(err, "Errore durante l'esecuzione di liqoctl connect", "output", output)
		_ = r.updateStatus(ctx, &connection, "Failed", fmt.Sprintf("Errore: %v, Output: %s", err, output))
		return ctrl.Result{}, err
	}

	logger.Info("Connessione riuscita", "nodeA", connection.Spec.VirtualNodeA, "nodeB", connection.Spec.VirtualNodeB)
	if err := r.updateStatus(ctx, &connection, "Connected", ""); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// executeLiqoctlConnect esegue il comando "liqoctl network connect" utilizzando i kubeconfig recuperati
func (r *VirtualNodeConnectionReconciler) executeLiqoctlConnect(ctx context.Context, connection *networkingv1alpha1.VirtualNodeConnection) (string, error) {
	kubeconfigA, err := r.getKubeconfigFromLiqo(ctx, connection.Spec.VirtualNodeA)
	if err != nil {
		return "", err
	}
	defer os.Remove(kubeconfigA)

	kubeconfigB, err := r.getKubeconfigFromLiqo(ctx, connection.Spec.VirtualNodeB)
	if err != nil {
		return "", err
	}
	defer os.Remove(kubeconfigB)

	ctxCmd, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxCmd, "liqoctl", "network", "connect",
		"--kubeconfig", kubeconfigA,
		"--remote-kubeconfig", kubeconfigB,
		"--server-service-type=NodePort")

	output, err := cmd.CombinedOutput()
	log.FromContext(ctx).Info("Output di liqoctl connect:" + string(output))

	if err != nil {
		return string(output), fmt.Errorf("Errore eseguendo liqoctl connect: %v", err)
	}

	return string(output), nil
}

// getKubeconfigFromLiqo recupera il kubeconfig dal Secret associato al virtual node
func (r *VirtualNodeConnectionReconciler) getKubeconfigFromLiqo(ctx context.Context, virtualNode string) (string, error) {
	namespace := fmt.Sprintf("liqo-tenant-%s", virtualNode)
	secretName := fmt.Sprintf("kubeconfig-%s", virtualNode)

	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, &secret); err != nil {
		return "", fmt.Errorf("Errore nel recupero del Secret %s nel namespace %s: %v", secretName, namespace, err)
	}

	kubeconfigData, exists := secret.Data["kubeconfig"]
	if !exists {
		return "", fmt.Errorf("Il Secret %s non contiene la chiave 'kubeconfig'", secretName)
	}

	kubeconfigPath := filepath.Join(os.TempDir(), fmt.Sprintf("kubeconfig-%s.yaml", virtualNode))
	if err := os.WriteFile(kubeconfigPath, kubeconfigData, 0600); err != nil {
		return "", fmt.Errorf("Errore nella scrittura del file kubeconfig: %v", err)
	}

	return kubeconfigPath, nil
}

// updateStatus aggiorna lo stato dell'oggetto VirtualNodeConnection utilizzando una patch per evitare conflitti
func (r *VirtualNodeConnectionReconciler) updateStatus(ctx context.Context, connection *networkingv1alpha1.VirtualNodeConnection, phase, errorMsg string) error {
	patch := client.MergeFrom(connection.DeepCopy())

	connection.Status.Phase = phase
	connection.Status.LastUpdated = time.Now().Format(time.RFC3339)
	connection.Status.ErrorMessage = errorMsg
	connection.Status.IsConnected = (phase == "Connected")

	if err := r.Status().Patch(ctx, connection, patch); err != nil {
		log.FromContext(ctx).Error(err, "Errore nell'aggiornamento dello stato")
		return err
	}
	return nil
}

// disconnectLiqoctl esegue il comando "liqoctl network disconnect" per disconnettere i nodi
func (r *VirtualNodeConnectionReconciler) disconnectLiqoctl(ctx context.Context, connection *networkingv1alpha1.VirtualNodeConnection) error {
	logger := log.FromContext(ctx)
	logger.Info("Avvio disconnessione", "name", connection.Name)

	kubeconfigA, err := r.getKubeconfigFromLiqo(ctx, connection.Spec.VirtualNodeA)
	if err != nil {
		return err
	}
	defer os.Remove(kubeconfigA)

	kubeconfigB, err := r.getKubeconfigFromLiqo(ctx, connection.Spec.VirtualNodeB)
	if err != nil {
		return err
	}
	defer os.Remove(kubeconfigB)

	ctxCmd, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxCmd, "liqoctl", "network", "disconnect",
		"--kubeconfig", kubeconfigA,
		"--remote-kubeconfig", kubeconfigB,
		"--skip-confirm")

	output, err := cmd.CombinedOutput()
	logger.Info("Output di liqoctl disconnect", "output", string(output))

	if err != nil {
		logger.Error(err, "Errore durante la disconnessione")
		return err
	}

	logger.Info("Disconnessione completata", "nodeA", connection.Spec.VirtualNodeA, "nodeB", connection.Spec.VirtualNodeB)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtualNodeConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha1.VirtualNodeConnection{}).
		Named("virtualnodeconnection").
		Complete(r)
}
