package main

import (
	"context"
	"fmt"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	// Create an in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(fmt.Errorf("failed to create in-cluster config: %v", err))
	}

	// Create Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(fmt.Errorf("failed to create Kubernetes client: %v", err))
	}

	// Define the daily schedule
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// for {
	// select {
	// case <-ticker.C:
	fmt.Println("Starting pod termination task...")
	err = terminateAllPods(clientset)
	if err != nil {
		fmt.Printf("Error terminating pods: %v\n", err)
	} else {
		fmt.Println("Successfully terminated all pods.")
	}
	// }
	// }
}

// terminateAllPods deletes all pods in all namespaces
func terminateAllPods(clientset *kubernetes.Clientset) error {
	// Get all namespaces
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %v", err)
	}

	// Loop through each namespace
	for _, namespace := range namespaces.Items {
		// Get all pods in the namespace
		pods, err := clientset.CoreV1().Pods(namespace.Name).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list pods in namespace %s: %v", namespace.Name, err)
		}

		// Delete each pod
		for _, pod := range pods.Items {
			describedPod, err := clientset.CoreV1().Pods(namespace.Name).Get(context.TODO(), pod.Name, metav1.GetOptions{})
			if err != nil {
				log.Fatalf("Failed to get pod: %v", err)
			}
			creationTimestamp := pod.CreationTimestamp.Time
			currentTime := time.Now()
			podAge := currentTime.Sub(creationTimestamp)
			maxPodAge := 24 * time.Hour
			if (podAge > maxPodAge) {
				fmt.Printf("OLD!!! Podname: %s CreationDate %s \n", pod.Name, &describedPod.OwnerReferences[0])

			} else {
				fmt.Printf("NEW!!! Podname: %s CreationDate %s \n", pod.Name, &describedPod.OwnerReferences[0])
			}
			
			// err := clientset.CoreV1().Pods(namespace.Name).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
			// if err != nil {
			// 	fmt.Printf("Failed to delete pod %s in namespace %s: %v\n", pod.Name, namespace.Name, err)
			// } else {
			// 	fmt.Printf("Deleted pod %s in namespace %s\n", pod.Name, namespace.Name)
			// }
		}
	}

	return nil
}
