package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	liqov1beta1 "virtualnode-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VirtualNodeConnectionReconciler gestisce le connessioni tra Virtual Nodes
type VirtualNodeConnectionReconciler struct {
	client.Client
}

// Reconcile gestisce la creazione e aggiornamento delle risorse VirtualNodeConnection
func (r *VirtualNodeConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var connection liqov1beta1.VirtualNodeConnection

	// Recupera la risorsa VirtualNodeConnection
	if err := r.Get(ctx, req.NamespacedName, &connection); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Se la connessione è già attiva, non fare nulla
	if connection.Status.IsConnected {
		return ctrl.Result{}, nil
	}

	fmt.Printf("🔹 Avvio connessione tra %s e %s\n", connection.Spec.VirtualNodeA, connection.Spec.VirtualNodeB)

	// Aggiorna lo stato a "Connecting"
	connection.Status.Phase = "Connecting"
	connection.Status.LastUpdated = time.Now().Format(time.RFC3339)
	_ = r.Status().Update(ctx, &connection)

	// Recupera i kubeconfig dai Secret
	kubeconfigA, err := r.getKubeconfig(ctx, connection.Spec.KubeconfigA)
	if err != nil {
		connection.Status.Phase = "Failed"
		connection.Status.ErrorMessage = fmt.Sprintf("Errore recupero kubeconfigA: %v", err)
		_ = r.Status().Update(ctx, &connection)
		return ctrl.Result{}, err
	}

	kubeconfigB, err := r.getKubeconfig(ctx, connection.Spec.KubeconfigB)
	if err != nil {
		connection.Status.Phase = "Failed"
		connection.Status.ErrorMessage = fmt.Sprintf("Errore recupero kubeconfigB: %v", err)
		_ = r.Status().Update(ctx, &connection)
		return ctrl.Result{}, err
	}

	// Salva i kubeconfig in file temporanei
	fileA := "/tmp/kubeconfig-a.yaml"
	fileB := "/tmp/kubeconfig-b.yaml"

	if err := os.WriteFile(fileA, []byte(kubeconfigA), 0644); err != nil {
		return ctrl.Result{}, err
	}
	if err := os.WriteFile(fileB, []byte(kubeconfigB), 0644); err != nil {
		return ctrl.Result{}, err
	}

	// Esegue `liqoctl network connect`
	cmd := exec.Command("liqoctl", "network", "connect",
		"--kubeconfig", fileA,
		"--remote-kubeconfig", fileB,
		"--server-service-type=NodePort")

	if err := cmd.Run(); err != nil {
		connection.Status.Phase = "Failed"
		connection.Status.ErrorMessage = fmt.Sprintf("Errore esecuzione liqoctl: %v", err)
		_ = r.Status().Update(ctx, &connection)
		return ctrl.Result{}, err
	}

	// Aggiorna lo stato della CRD a "Connected"
	connection.Status.IsConnected = true
	connection.Status.Phase = "Connected"
	connection.Status.LastUpdated = time.Now().Format(time.RFC3339)
	connection.Status.ErrorMessage = ""
	if err := r.Status().Update(ctx, &connection); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// getKubeconfig recupera una kubeconfig da un Secret Kubernetes
func (r *VirtualNodeConnectionReconciler) getKubeconfig(ctx context.Context, secretName string) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: "kube-system"}, &secret); err != nil {
		return "", err
	}
	kubeconfig, exists := secret.Data["kubeconfig"]
	if !exists {
		return "", fmt.Errorf("chiave 'kubeconfig' non trovata nel Secret %s", secretName)
	}
	return string(kubeconfig), nil
}

// SetupWithManager imposta il controller nel manager
func (r *VirtualNodeConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&liqov1beta1.VirtualNodeConnection{}).
		Complete(r)
}

func main() {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{})
	if err != nil {
		os.Exit(1)
	}

	if err := (&VirtualNodeConnectionReconciler{Client: mgr.GetClient()}).SetupWithManager(mgr); err != nil {
		os.Exit(1)
	}

	mgr.Start(ctrl.SetupSignalHandler())
}

