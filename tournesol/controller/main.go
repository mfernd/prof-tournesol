package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type diagnostic struct {
	name     string
	kind     string
	error    string
	solution string
}

// handleResult processes and displays Result information
func handleResult(obj *unstructured.Unstructured) {
	var diag diagnostic
	diag.name, _, _ = unstructured.NestedString(obj.Object, "spec", "name")
	diag.kind, _, _ = unstructured.NestedString(obj.Object, "spec", "kind")

	details, _, _ := unstructured.NestedString(obj.Object, "spec", "details")
	parts := strings.SplitN(details, "\n\n", 2)
	if len(parts) >= 1 {
		diag.error = strings.TrimPrefix(parts[0], "Error: ")
	}
	if len(parts) >= 2 {
		diag.solution = strings.TrimPrefix(parts[1], "Solution: ")
	}

	// For now, it just prints the diagnostic information
	fmt.Printf("[%s]\nKind: %s\nError: %s\nSolution: %s\n", diag.name, diag.kind, diag.error, diag.solution)
}

func main() {
	// Load kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Printf("Error building kubeconfig: %v\n", err)
		os.Exit(1)
	}

	// Create dynamic client for CRDs
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error creating dynamic client: %v\n", err)
		os.Exit(1)
	}

	// Define the GVR for Result resources
	gvr := schema.GroupVersionResource{
		Group:    "core.k8sgpt.ai",
		Version:  "v1alpha1",
		Resource: "results",
	}

	// Create dynamic informer factory
	namespace := "k8sgpt-operator-system"
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynamicClient,
		time.Minute*30,
		namespace,
		nil,
	)

	// Get informer for Result resources
	informer := factory.ForResource(gvr).Informer()

	// Set up event handlers
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			handleResult(obj.(*unstructured.Unstructured))
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			handleResult(newObj.(*unstructured.Unstructured))
		},
	})

	// Set up signal handling
	stopCh := make(chan struct{})
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalCh
		fmt.Println("Shutting down Prof Tournesol controller...")
		close(stopCh)
	}()

	fmt.Printf("Prof Tournesol controller started - watching namespace '%s'\n", namespace)

	// Start the informer and wait
	go informer.Run(stopCh)

	// Wait for the stop signal
	<-stopCh
}
