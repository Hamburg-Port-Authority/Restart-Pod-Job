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
	// Create Kubernetes client
	clientset, err := getKubernetesClient()
	if err != nil {
		log.Fatalf("error initializing Kubernetes client: %v", err)
	}

	log.Println("Starting pod termination task...")
	if err := terminateAllPods(clientset); err != nil {
		log.Printf("Error terminating pods: %v", err)
	} else {
		log.Println("Successfully terminated all pods.")
	}
}

func getKubernetesClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(fmt.Errorf("failed to create Kubernetes client: %v", err))
	}
	return kubernetes.NewForConfig(config)
}

// terminateAllPods deletes all pods in all namespaces
func terminateAllPods(clientset *kubernetes.Clientset) error {

	currentTime := time.Now()
	lastRestartedNamespace, lastRestartedResource = "", ""

	// Get all namespaces
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %v", err)
	}
	for _, namespace := range namespaces.Items {
		handleNamespace(clientset, namespace, currentTime)
	}
	return nil
}

func handleNamespace(clientset *kubernetes.Clientset, namespace v1.Namespace, currentTime time.Time) error {
	//get all pods in current namespace
	pods, err := clientset.CoreV1().Pods(namespace.Name).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace %s: %v", namespace.Name, err)
	}
	for _, pod := range pods.Items {
		// check annotations of ns if ttl-annotation exists
		err1 := handlePod(pod, namespace, currentTime, clientset)
		if err1 != nil {
			return err1
		}
	}
	return nil
}

func handlePod(pod v1.Pod, namespace v1.Namespace, currentTime time.Time, clientset *kubernetes.Clientset) error {
	ttl, exists := pod.Annotations["restart.k8s.hpa.de/ttl"]
	if !exists {
		log.Printf("Pod %s will not be restarted -> no annotation", pod.Name)
		return nil
	}
	// ttl exists -> cast into duration
	ttlInDuration, err := str2duration.ParseDuration(ttl)
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace %s: %v", namespace.Name, err)
	}
	podAge := currentTime.Sub(pod.CreationTimestamp.Time)

	// if pod is older than ttl
	if podAge < ttlInDuration {
		log.Printf("Pod %s will not be restarted -> not old enough; age: %s; ttl: %s", pod.Name, podAge, ttlInDuration)
		return nil
	}
	err = restartPodOwner(namespace.Name, pod.Name, clientset)
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace %s: %v", namespace.Name, err)
	}
	return nil
}

func restartPodOwner(namespaceName string, podName string, clientset *kubernetes.Clientset) error {
	// describe pod to be restarted
	describedPod, err := clientset.CoreV1().Pods(namespaceName).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		log.Printf("failed to get pod %s in namespace %s: %v", podName, namespaceName, err)
		return nil
	}
	//check if pod has owner
	if (describedPod.OwnerReferences == nil) || (len(describedPod.OwnerReferences) == 0) {
		log.Printf("Pod %s has no Owner -> would be deleted permanently", podName)
		return nil
	}

	if (describedPod.OwnerReferences[0].Name == lastRestartedResource) && (namespaceName == lastRestartedNamespace) {
		log.Printf("Kind %s name %s is already being restarted", describedPod.OwnerReferences[0].Kind, describedPod.OwnerReferences[0].Name)
		return nil
	}

	switch describedPod.OwnerReferences[0].Kind {
	case "ReplicaSet":
		//get Deployment to restart
		return handleReplicaSet(clientset, namespaceName, describedPod)
	case "DaemonSet":
	case "StatefulSet":
		return restartResource(clientset, namespaceName, describedPod.OwnerReferences[0].Name, describedPod.OwnerReferences[0].Kind)
	}
	return nil
}

func handleReplicaSet(clientset *kubernetes.Clientset, namespaceName string, describedPod *v1.Pod) error {
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
	return restartResource(clientset, namespaceName, describedRS.OwnerReferences[0].Name, describedRS.OwnerReferences[0].Kind)
}

func restartResource(clientset *kubernetes.Clientset, namespace, resourceName, resourceType string) error {
	log.Printf("Resource %s %s is being restarted", resourceType, resourceName)
	switch resourceType {
	case "Deployment":
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), resourceName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get Deployment %s: %w", resourceName, err)
		}
		if deployment.GetAnnotations() == nil {
			deployment.SetAnnotations(make(map[string]string))
		}
		deployment.GetAnnotations()["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
		_, err = clientset.AppsV1().Deployments(namespace).Update(context.TODO(), deployment, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update %s %s: %w", resourceType, resourceName, err)
		}
	case "DaemonSet":
		daemonSet, err := clientset.AppsV1().DaemonSets(namespace).Get(context.TODO(), resourceName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get DaemonSet %s: %w", resourceName, err)
		}
		if daemonSet.GetAnnotations() == nil {
			daemonSet.SetAnnotations(make(map[string]string))
		}
		daemonSet.GetAnnotations()["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
		_, err = clientset.AppsV1().DaemonSets(namespace).Update(context.TODO(), daemonSet, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update %s %s: %w", resourceType, resourceName, err)
		}
	case "StatefulSet":
		statefulSet, err := clientset.AppsV1().StatefulSets(namespace).Get(context.TODO(), resourceName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get StatefulSet %s: %w", resourceName, err)
		}
		if statefulSet.GetAnnotations() == nil {
			statefulSet.SetAnnotations(make(map[string]string))
		}
		statefulSet.GetAnnotations()["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
		_, err = clientset.AppsV1().StatefulSets(namespace).Update(context.TODO(), statefulSet, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update %s %s: %w", resourceType, resourceName, err)
		}
	}

	lastRestartedResource = resourceName
	lastRestartedNamespace = namespace
	log.Printf("Resource %s in namespace %s has been restarted", lastRestartedResource, lastRestartedNamespace)
	return nil
}
