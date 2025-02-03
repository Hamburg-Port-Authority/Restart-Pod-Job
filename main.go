package main

import (
	"context"
	"fmt"
	"log"

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

	fmt.Println("Starting pod termination task...")
	err = terminateAllPods(clientset)
	if err != nil {
		fmt.Printf("Error terminating pods: %v\n", err)
	} else {
		fmt.Println("Successfully terminated all pods.")
	}
}

// terminateAllPods deletes all pods in all namespaces
func terminateAllPods(clientset *kubernetes.Clientset) error {

	// currentTime := time.Now()
	// lastRestartedResource := ""
	// lastRestartedNamespace := ""

	// Get all namespaces
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %v", err)
	}
	for _, namespace := range namespaces.Items {
		// describe ns
		describedNs, err := clientset.CoreV1().Namespaces().Get(context.TODO(), namespace.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to list namespaces: %v", err)
		}
		ttl, exists := describedNs.ObjectMeta.Annotations["ttl"]
		if (exists) {
			log.Printf("ttl: %s", ttl)
		} else {
			log.Printf("%s has no ttl", &namespace.Name)
		}
	}

	// // Loop through each namespace
	// for _, namespace := range namespaces.Items {
	// 	// Get all pods in the namespace
	// 	pods, err := clientset.CoreV1().Pods(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	// 	if err != nil {
	// 		return fmt.Errorf("failed to list pods in namespace %s: %v", namespace.Name, err)
	// 	}

	// 	// Delete each pod
	// 	for _, pod := range pods.Items {

	// 		describedPod, err := clientset.CoreV1().Pods(namespace.Name).Get(context.TODO(), pod.Name, metav1.GetOptions{})
	// 		if err != nil {
	// 			log.Printf("Failed to get pod: %v", err)
	// 		}
	// 		if describedPod.ObjectMeta.Annotations != nil && describedPod.ObjectMeta.Annotations["ttl"] != nil{
	// 			ttl := describedPod.ObjectMeta.Annotations["ttl"]
	// 		}
	// 		podAge := currentTime.Sub(pod.CreationTimestamp.Time)

	// 		if podAge > maxPodAge {
	// 			kindOfOwner := describedPod.OwnerReferences[0].Kind
	// 			nameOfOwner := describedPod.OwnerReferences[0].Name
	// 			if kindOfOwner == "ReplicaSet" {
	// 				describedRS, err := clientset.AppsV1().ReplicaSets(namespace.Name).Get(context.TODO(), nameOfOwner, metav1.GetOptions{})
	// 				if err != nil {
	// 					log.Printf("Failed to get replicaset %s: %v", nameOfOwner, err)
	// 				} else {
	// 					nameofDeployment := describedRS.OwnerReferences[0].Name
	// 					describedDeploy, err := clientset.AppsV1().Deployments(namespace.Name).Get(context.TODO(), nameofDeployment, metav1.GetOptions{})
	// 					if err != nil {
	// 						log.Fatalf("Failed to get replicaset %s: %v", nameOfOwner, err)
	// 					}
	// 					if (nameofDeployment != lastRestartedResource) || (namespace.Name != lastRestartedNamespace) {
	// 						// Update the deployment annotation to trigger a rollout restart
	// 						if describedDeploy.Spec.Template.ObjectMeta.Annotations == nil {
	// 							describedDeploy.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	// 						}
	// 						describedDeploy.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	// 						// Apply the update
	// 						_, err = clientset.AppsV1().Deployments(namespace.Name).Update(context.TODO(), describedDeploy, metav1.UpdateOptions{})
	// 						if err != nil {
	// 							log.Fatalf("Failed to update deployment: %v", err)
	// 						}
	// 						lastRestartedResource = nameofDeployment
	// 						lastRestartedNamespace = namespace.Name
	// 					}
	// 				}
	// 			} else if kindOfOwner == "StatefulSet" {
	// 				if (nameOfOwner != lastRestartedResource) || (namespace.Name != lastRestartedNamespace) {
	// 					describedSts, err := clientset.AppsV1().StatefulSets(namespace.Name).Get(context.TODO(), nameOfOwner, metav1.GetOptions{})
	// 					if err != nil {
	// 						log.Fatalf("Failed to describe sts: %v", err)
	// 					}
	// 					// Update the deployment annotation to trigger a rollout restart
	// 					if describedSts.Spec.Template.ObjectMeta.Annotations == nil {
	// 						describedSts.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	// 					}
	// 					describedSts.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
	// 					_, err = clientset.AppsV1().StatefulSets(namespace.Name).Update(context.TODO(), describedSts, metav1.UpdateOptions{})
	// 					if err != nil {
	// 						log.Fatalf("Failed to update sts: %v", err)
	// 					}
	// 					lastRestartedResource = nameOfOwner
	// 					lastRestartedNamespace = namespace.Name
	// 				}
	// 			} else if kindOfOwner == "DaemonSet" {
	// 				if (nameOfOwner != lastRestartedResource) || (namespace.Name != lastRestartedNamespace) {
	// 					describedDs, err := clientset.AppsV1().DaemonSets(namespace.Name).Get(context.TODO(), nameOfOwner, metav1.GetOptions{})
	// 					if err != nil {
	// 						log.Fatalf("Failed to describe ds: %v", err)
	// 					}
	// 					// Update the deployment annotation to trigger a rollout restart
	// 					if describedDs.Spec.Template.ObjectMeta.Annotations == nil {
	// 						describedDs.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	// 					}
	// 					describedDs.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
	// 					_, err = clientset.AppsV1().DaemonSets(namespace.Name).Update(context.TODO(), describedDs, metav1.UpdateOptions{})
	// 					if err != nil {
	// 						log.Fatalf("Failed to update ds: %v", err)
	// 					}
	// 					lastRestartedResource = nameOfOwner
	// 					lastRestartedNamespace = namespace.Name
	// 				}
	// 			}
	// 			// kubectl rollout restart kindofOwner <daemonset-name> -n <namespace>
	// 			fmt.Printf("OLD!!! Podname: %s; age: %s; OwnerName %s; OwnerKind %s\n", pod.Name, podAge, nameOfOwner, kindOfOwner)
	// 		} else {
	// 			fmt.Printf("NEW!!! Podname: %s CreationDate %s \n", pod.Name, &describedPod.OwnerReferences[0])
	// 		}
	// 	}
	// }

	return nil
}
