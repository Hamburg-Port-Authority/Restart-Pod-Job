package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/xhit/go-str2duration/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var lastRestartedNamespace, lastRestartedResource string

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

	currentTime := time.Now()
	lastRestartedResource = ""
	lastRestartedNamespace = ""

	// Get all namespaces
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %v", err)
	}
	for _, namespace := range namespaces.Items {
		// describe ns
		describedNs, err := clientset.CoreV1().Namespaces().Get(context.TODO(), namespace.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to describe namespace: %v", err)
		}
		// check annotations of ns if ttl-annotation exists
		ttl, exists := describedNs.ObjectMeta.Annotations["ttl"]
		if exists {
			// ttl exists -> cast into duration
			ttlInDuration, err := str2duration.ParseDuration(ttl)
			if err != nil {
				return fmt.Errorf("failed to list pods in namespace %s: %v", namespace.Name, err)
			}
			//get all pods in current namespace
			pods, err := clientset.CoreV1().Pods(namespace.Name).List(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return fmt.Errorf("failed to list pods in namespace %s: %v", namespace.Name, err)
			}
			for _, pod := range pods.Items {
				podAge := currentTime.Sub(pod.CreationTimestamp.Time)
				// if pod is older than ttl
				if ttlInDuration < podAge {
					err := restartPodOwner(namespace.Name, pod.Name, clientset)
					if err != nil {
						return fmt.Errorf("failed to list pods in namespace %s: %v", namespace.Name, err)
					}
				}
			}
		}
	}
	return nil
}

func restartPodOwner(namespaceName string, podName string, clientset *kubernetes.Clientset) error {
	// describe pod to be restarted
	describedPod, err := clientset.CoreV1().Pods(namespaceName).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace %s: %v", namespaceName, err)
	}
	//check if pod has owner
	if describedPod.OwnerReferences == nil {
		log.Printf("Pod %s has no Owner -> would be deleted permanently", podName)
		return nil
	}
	if describedPod.OwnerReferences[0].Kind == "ReplicaSet" {
		err := restartDeployment(clientset, namespaceName, describedPod)
		if err != nil {
			return fmt.Errorf("failed to restart deployments in namespace %s: %v", namespaceName, err)
		}
	} else if describedPod.OwnerReferences[0].Kind == "DaemonSet" {
		err := restartDaemonSet(clientset, namespaceName, describedPod)
		if err != nil {
			return fmt.Errorf("failed to restart daemonsets in namespace %s: %v", namespaceName, err)
		}
	} else if describedPod.OwnerReferences[0].Kind == "StatefulSet" {
		err := restartStatefulSet(clientset, namespaceName, describedPod)
		if err != nil {
			return fmt.Errorf("failed to restart statefulsets in namespace %s: %v", namespaceName, err)
		}
	}

	return nil
}

func restartDeployment(clientset *kubernetes.Clientset, namespaceName string, describedPod *v1.Pod) error {
	describedRS, err := clientset.AppsV1().ReplicaSets(namespaceName).Get(context.TODO(), describedPod.OwnerReferences[0].Name, metav1.GetOptions{})
	if err != nil {
		log.Printf("Failed to get replicaset %s: %v", describedPod.OwnerReferences[0].Name, err)
		return nil
	}
	//check if Rs has owner
	if describedRS.OwnerReferences == nil {
		log.Printf("ReplicaSet %s has no Owner -> would be deleted permanently", describedRS.Name)
		return nil
	}
	// Describe Deployment -> Owner of Rs
	describedDeploy, err := clientset.AppsV1().Deployments(namespaceName).Get(context.TODO(), describedRS.OwnerReferences[0].Name, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("Failed to get deplyoment %s: %v", describedRS.OwnerReferences[0].Name, err)
	}
	if (describedDeploy.Name == lastRestartedResource) && (namespaceName == lastRestartedNamespace) {
		log.Printf("Deployment %s is already being restarted", describedDeploy.Name)
		return nil
	}
	// Update the deployment annotation to trigger a rollout restart
	if describedDeploy.Spec.Template.ObjectMeta.Annotations == nil {
		describedDeploy.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	describedDeploy.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	// Apply the update
	_, err = clientset.AppsV1().Deployments(namespaceName).Update(context.TODO(), describedDeploy, metav1.UpdateOptions{})
	if err != nil {
		log.Fatalf("Failed to update deployment: %v", err)
	}
	//sets lastRestartedResource as current deployment
	lastRestartedResource = describedDeploy.Name
	//sets lastRestartedNamespace as current namespace
	lastRestartedNamespace = namespaceName
	return nil
}

func restartDaemonSet(clientset *kubernetes.Clientset, namespaceName string, describedPod *v1.Pod) error {
	if (describedPod.OwnerReferences[0].Name == lastRestartedResource) && (namespaceName == lastRestartedNamespace) {
		log.Printf("DaemonSet %s is already being restarted", describedPod.OwnerReferences[0].Name)
		return nil
	}

	describedDs, err := clientset.AppsV1().DaemonSets(namespaceName).Get(context.TODO(), describedPod.OwnerReferences[0].Name, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("Failed to describe ds: %v", err)
	}
	// Update the deployment annotation to trigger a rollout restart
	if describedDs.Spec.Template.ObjectMeta.Annotations == nil {
		describedDs.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	describedDs.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
	_, err = clientset.AppsV1().DaemonSets(namespaceName).Update(context.TODO(), describedDs, metav1.UpdateOptions{})
	if err != nil {
		log.Fatalf("Failed to update ds: %v", err)
	}
	//sets lastRestartedResource as current daemonset
	lastRestartedResource = describedDs.Name
	//sets lastRestartedNamespace as current namespace
	lastRestartedNamespace = namespaceName
	return nil
}

func restartStatefulSet(clientset *kubernetes.Clientset, namespaceName string, describedPod *v1.Pod) error {
	if (describedPod.OwnerReferences[0].Name == lastRestartedResource) && (namespaceName == lastRestartedNamespace) {
		log.Printf("StatefulSet %s is already being restarted", describedPod.OwnerReferences[0].Name)
		return nil
	}

	describedSts, err := clientset.AppsV1().StatefulSets(namespaceName).Get(context.TODO(), describedPod.OwnerReferences[0].Name, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("Failed to describe ds: %v", err)
	}
	// Update the deployment annotation to trigger a rollout restart
	if describedSts.Spec.Template.ObjectMeta.Annotations == nil {
		describedSts.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	describedSts.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
	_, err = clientset.AppsV1().StatefulSets(namespaceName).Update(context.TODO(), describedSts, metav1.UpdateOptions{})
	if err != nil {
		log.Fatalf("Failed to update ds: %v", err)
	}
	//sets lastRestartedResource as current daemonset
	lastRestartedResource = describedSts.Name
	//sets lastRestartedNamespace as current namespace
	lastRestartedNamespace = namespaceName
	return nil
}
