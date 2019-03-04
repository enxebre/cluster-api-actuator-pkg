package infra

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/golang/glog"
	e2e "github.com/openshift/cluster-api-actuator-pkg/pkg/e2e/framework"
	mapiv1beta1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	controllernode "github.com/openshift/cluster-api/pkg/controller/node"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nodeWorkerRoleLabel        = "node-role.kubernetes.io/worker"
	deprecatedMachineRoleLabel = "sigs.k8s.io/cluster-api-machine-role"
	machineRoleLabel           = "machine.openshift.io/cluster-api-machine-role"
	machineAPIGroup            = "machine.openshift.io"
)

func isOneMachinePerNode(client runtimeclient.Client) bool {
	listOptions := runtimeclient.ListOptions{
		Namespace: e2e.TestContext.MachineApiNamespace,
	}
	machineList := mapiv1beta1.MachineList{}
	nodeList := corev1.NodeList{}

	if err := wait.PollImmediate(5*time.Second, e2e.WaitMedium, func() (bool, error) {
		if err := client.List(context.TODO(), &listOptions, &machineList); err != nil {
			glog.Errorf("Error querying api for machineList object: %v, retrying...", err)
			return false, nil
		}
		if err := client.List(context.TODO(), &listOptions, &nodeList); err != nil {
			glog.Errorf("Error querying api for nodeList object: %v, retrying...", err)
			return false, nil
		}

		glog.Infof("Expecting the same number of machines and nodes, have %d nodes and %d machines", len(nodeList.Items), len(machineList.Items))
		if len(machineList.Items) != len(nodeList.Items) {
			return false, nil
		}

		nodeNameToMachineAnnotation := make(map[string]string)
		for _, node := range nodeList.Items {
			if _, ok := node.Annotations[controllernode.MachineAnnotationKey]; !ok {
				glog.Errorf("Node %q does not have a MachineAnnotationKey %q, retrying...", node.Name, controllernode.MachineAnnotationKey)
				return false, nil
			}
			nodeNameToMachineAnnotation[node.Name] = node.Annotations[controllernode.MachineAnnotationKey]
		}
		for _, machine := range machineList.Items {
			if machine.Status.NodeRef == nil {
				glog.Errorf("Machine %q has no NodeRef, retrying...", machine.Name)
				return false, nil
			}
			nodeName := machine.Status.NodeRef.Name
			if nodeNameToMachineAnnotation[nodeName] != fmt.Sprintf("%s/%s", e2e.TestContext.MachineApiNamespace, machine.Name) {
				glog.Errorf("Node name %q does not match expected machine name %q, retrying...", nodeName, machine.Name)
				return false, nil
			}
			glog.Infof("Machine %q is linked to node %q", machine.Name, nodeName)
		}
		return true, nil
	}); err != nil {
		glog.Errorf("Error checking isOneMachinePerNode: %v", err)
		return false
	}
	return true
}

// getClusterSize returns the number of nodes of the cluster
func getClusterSize(client runtimeclient.Client) (*int, error) {
	nodeList := corev1.NodeList{}
	var size int
	if err := wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
		if err := client.List(context.TODO(), &runtimeclient.ListOptions{}, &nodeList); err != nil {
			glog.Errorf("Error querying api for nodeList object: %v, retrying...", err)
			return false, nil
		}
		size = len(nodeList.Items)
		return true, nil
	}); err != nil {
		glog.Errorf("Error calling getClusterSize: %v", err)
		return nil, err
	}
	glog.Infof("Cluster size is %d nodes", size)
	return &size, nil
}

func getWorkerNode(client runtimeclient.Client) (*corev1.Node, error) {
	nodeList := corev1.NodeList{}
	listOptions := runtimeclient.ListOptions{}
	listOptions.MatchingLabels(map[string]string{nodeWorkerRoleLabel: ""})
	if err := wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
		if err := client.List(context.TODO(), &listOptions, &nodeList); err != nil {
			glog.Errorf("Error querying api for nodeList object: %v, retrying...", err)
			return false, nil
		}
		if len(nodeList.Items) < 1 {
			glog.Errorf("No nodes were found with label %q", nodeWorkerRoleLabel)
			return false, nil
		}
		return true, nil
	}); err != nil {
		glog.Errorf("Error calling getWorkerMachine: %v", err)
		return nil, err
	}
	return &nodeList.Items[0], nil
}

func getMachineFromNode(client runtimeclient.Client, node *corev1.Node) (*mapiv1beta1.Machine, error) {
	machineNamespaceKey, ok := node.Annotations[controllernode.MachineAnnotationKey]
	if !ok {
		return nil, fmt.Errorf("node %q does not have a MachineAnnotationKey %q", node.Name, controllernode.MachineAnnotationKey)
	}
	namespace, machineName, err := cache.SplitMetaNamespaceKey(machineNamespaceKey)
	if err != nil {
		return nil, fmt.Errorf("machine annotation format is incorrect %v: %v", machineNamespaceKey, err)
	}

	key := runtimeclient.ObjectKey{Namespace: namespace, Name: machineName}
	machine := mapiv1beta1.Machine{}
	if err := wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
		if err := client.Get(context.TODO(), key, &machine); err != nil {
			glog.Errorf("Error querying api for nodeList object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		glog.Errorf("Error calling getMachineFromNode: %v", err)
		return nil, err
	}
	return &machine, nil
}

func getMachineSetFromMachine(client runtimeclient.Client, machine mapiv1beta1.Machine) (*mapiv1beta1.MachineSet, error) {
	for key := range machine.OwnerReferences {
		if machine.OwnerReferences[key].Kind == "MachineSet" {
			machineSet := mapiv1beta1.MachineSet{}
			key := runtimeclient.ObjectKey{Namespace: e2e.TestContext.MachineApiNamespace, Name: machine.OwnerReferences[key].Name}
			if err := wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
				if err := client.Get(context.TODO(), key, &machineSet); err != nil {
					glog.Errorf("error querying api for machineSet object: %v, retrying...", err)
					return false, nil
				}
				return true, nil
			}); err != nil {
				glog.Errorf("Error calling getMachineSetFromMachine: %v", err)
				return nil, err
			}
			return &machineSet, nil
		}
	}
	return nil, fmt.Errorf("no MachineSet found for machine %q", machine.Name)
}

func getMachineSetByName(client runtimeclient.Client, name string) (*mapiv1beta1.MachineSet, error) {
	machineSet := mapiv1beta1.MachineSet{}
	key := runtimeclient.ObjectKey{Namespace: e2e.TestContext.MachineApiNamespace, Name: name}
	if err := wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
		if err := client.Get(context.TODO(), key, &machineSet); err != nil {
			glog.Errorf("error querying api for machineSet object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		glog.Errorf("Error calling getMachineByName: %v", err)
		return nil, err
	}
	return &machineSet, nil
}

func getMachineSetList(client runtimeclient.Client) (*mapiv1beta1.MachineSetList, error) {
	machineSetList := mapiv1beta1.MachineSetList{}
	listOptions := runtimeclient.ListOptions{
		Namespace: e2e.TestContext.MachineApiNamespace,
	}
	if err := wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
		if err := client.List(context.TODO(), &listOptions, &machineSetList); err != nil {
			glog.Errorf("error querying api for machineSetList object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		glog.Errorf("Error calling getMachineSetList: %v", err)
		return nil, err
	}
	return &machineSetList, nil
}

func getMachineSetListWorkers(client runtimeclient.Client) (*mapiv1beta1.MachineSetList, error) {
	machineSetList := mapiv1beta1.MachineSetList{}
	listOptions := runtimeclient.ListOptions{
		Namespace: e2e.TestContext.MachineApiNamespace,
	}
	listOptions.MatchingLabels(map[string]string{deprecatedMachineRoleLabel: "worker"})
	if err := wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
		if err := client.List(context.TODO(), &listOptions, &machineSetList); err != nil {
			glog.Errorf("error querying api for machineSetList object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		glog.Errorf("Error calling getMachineSetListWorkers: %v", err)
		return nil, err
	}
	return &machineSetList, nil
}

func getMachineList(client runtimeclient.Client) (*mapiv1beta1.MachineList, error) {
	machineList := mapiv1beta1.MachineList{}
	listOptions := runtimeclient.ListOptions{
		Namespace: e2e.TestContext.MachineApiNamespace,
	}
	if err := wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
		if err := client.List(context.TODO(), &listOptions, &machineList); err != nil {
			glog.Errorf("error querying api for machineList object: %v, retrying...", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		glog.Errorf("Error calling getMachineList: %v", err)
		return nil, err
	}
	return &machineList, nil
}

func waitUntilAllNodesAreSchedulable(client runtimeclient.Client) error {
	return wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
		nodeList := corev1.NodeList{}
		if err := client.List(context.TODO(), &runtimeclient.ListOptions{}, &nodeList); err != nil {
			glog.Errorf("error querying api for nodeList object: %v, retrying...", err)
			return false, nil
		}
		// All nodes needs to be ready
		for _, node := range nodeList.Items {
			if node.Spec.Unschedulable == true {
				glog.Errorf("Node %q is unschedulable", node.Name)
				return false, nil
			}
			glog.Infof("Node %q is schedulable", node.Name)
		}
		return true, nil
	})
}

func getScaleClient() (scale.ScalesGetter, error) {
	cfg, err := e2e.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("error getting config %v", err)
	}
	mapper, err := apiutil.NewDiscoveryRESTMapper(cfg)
	if err != nil {
		return nil, fmt.Errorf("error calling NewDiscoveryRESTMapper %v", err)
	}

	discovery := discovery.NewDiscoveryClientForConfigOrDie(cfg)
	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(discovery)
	scaleClient, err := scale.NewForConfig(cfg, mapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
	if err != nil {
		return nil, fmt.Errorf("error calling building scale client %v", err)
	}
	return scaleClient, nil
}

// scaleAWorker finds a worker machineSet and scales it to the given number of replicas
func scaleAWorker(client runtimeclient.Client, replicas int) error {
	workerNode, err := getWorkerNode(client)
	if err != nil {
		return fmt.Errorf("error calling getWorkerNode %v", err)
	}
	glog.Infof("Got workerNode %q", workerNode.Name)

	workerMachine, err := getMachineFromNode(client, workerNode)
	if err != nil {
		return fmt.Errorf("error calling getMachineFromNode %v", err)
	}
	glog.Infof("Got workerMachine %q", workerMachine.Name)

	workerMachineSet, err := getMachineSetFromMachine(client, *workerMachine)
	if err != nil {
		return fmt.Errorf("error calling getMachineSetFromMachine %v", err)
	}
	glog.Infof("Got workerMachineSet %q", workerMachineSet.Name)

	scaleClient, err := getScaleClient()
	if err != nil {
		return fmt.Errorf("error calling getScaleClient %v", err)
	}

	scale, err := scaleClient.Scales(e2e.TestContext.MachineApiNamespace).Get(schema.GroupResource{Group: machineAPIGroup, Resource: "MachineSet"}, workerMachineSet.Name)
	if err != nil {
		return fmt.Errorf("error calling scaleClient.Scales get: %v", err)
	}

	scaleUpdate := scale.DeepCopy()
	scaleUpdate.Spec.Replicas = int32(replicas)
	_, err = scaleClient.Scales(e2e.TestContext.MachineApiNamespace).Update(schema.GroupResource{Group: machineAPIGroup, Resource: "MachineSet"}, scaleUpdate)
	if err != nil {
		return fmt.Errorf("error calling scaleClient.Scales update: %v", err)
	}

	glog.Infof("%q original replicas: %d. Scaling to: %d", workerMachineSet.Name, *workerMachineSet.Spec.Replicas, replicas)
	return nil
}

func getMachineSet(client runtimeclient.Client, name string) (*mapiv1beta1.MachineSet, error) {
	machineSet := &mapiv1beta1.MachineSet{}
	key := runtimeclient.ObjectKey{Namespace: e2e.TestContext.MachineApiNamespace, Name: name}
	if err := wait.PollImmediate(1*time.Second, time.Minute, func() (bool, error) {
		if err := client.Get(context.TODO(), key, machineSet); err != nil {
			glog.Errorf("error querying api for machineSet %q: %v, retrying...", name, err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, fmt.Errorf("error calling getMachineSet: %v", err)
	}
	return machineSet, nil
}

func scaleMachineSet(name string, replicas int) error {
	scaleClient, err := getScaleClient()
	if err != nil {
		return fmt.Errorf("error calling getScaleClient %v", err)
	}

	scale, err := scaleClient.Scales(e2e.TestContext.MachineApiNamespace).Get(schema.GroupResource{Group: machineAPIGroup, Resource: "MachineSet"}, name)
	if err != nil {
		return fmt.Errorf("error calling scaleClient.Scales get: %v", err)
	}

	scaleUpdate := scale.DeepCopy()
	scaleUpdate.Spec.Replicas = int32(replicas)
	_, err = scaleClient.Scales(e2e.TestContext.MachineApiNamespace).Update(schema.GroupResource{Group: machineAPIGroup, Resource: "MachineSet"}, scaleUpdate)
	if err != nil {
		return fmt.Errorf("error calling scaleClient.Scales update: %v", err)
	}
	return nil
}

// areNodesReady returns true if an array of nodes are all ready
func areNodesReady(nodes []*corev1.Node) bool {
	// All nodes needs to be ready
	for key := range nodes {
		if !e2e.IsNodeReady(nodes[key]) {
			glog.Errorf("Node %q is not ready. Conditions are: %v", nodes[key].Name, nodes[key].Status.Conditions)
			return false
		}
		glog.Infof("Node %q is ready. Conditions are: %v", nodes[key].Name, nodes[key].Status.Conditions)
	}
	return true
}

// getMachinesFromMachineSet returns an array of machines owned by a fiven machineSet
func getMachinesFromMachineSet(client runtimeclient.Client, machineSet mapiv1beta1.MachineSet) ([]mapiv1beta1.Machine, error) {
	machineList, err := getMachineList(client)
	if err != nil {

	}
	var machinesForSet []mapiv1beta1.Machine
	for key := range machineList.Items {
		if metav1.IsControlledBy(&machineList.Items[key], &machineSet) {
			machinesForSet = append(machinesForSet, machineList.Items[key])
		}
	}
	return machinesForSet, nil
}

// getNodesFromMachineSet returns an array of nodes backed by machines owned by a given machineSet
func getNodesFromMachineSet(client runtimeclient.Client, machineSet mapiv1beta1.MachineSet) ([]*corev1.Node, error) {
	machines, err := getMachinesFromMachineSet(client, machineSet)
	if err != nil {
		return nil, fmt.Errorf("error calling getMachinesFromMachineSet %v", err)
	}

	var nodes []*corev1.Node
	for key := range machines {
		node, err := getNodeFromMachine(client, machines[key])
		if err != nil {
			return nil, fmt.Errorf("error getting node from machine %q: %v", machines[key].Name, err)
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

// getNodeFromMachine returns the node object referenced by machine.Status.NodeRef
func getNodeFromMachine(client runtimeclient.Client, machine mapiv1beta1.Machine) (*corev1.Node, error) {
	var node corev1.Node
	if machine.Status.NodeRef != nil {
		key := runtimeclient.ObjectKey{Namespace: machine.Status.NodeRef.Namespace, Name: machine.Status.NodeRef.Name}
		if err := client.Get(context.Background(), key, &node); err != nil {
			return nil, fmt.Errorf("error getting node %q: %v", node.Name, err)
		}
	}
	return &node, nil
}

func machineSetsSnapShot(client runtimeclient.Client) error {
	machineSetList, err := getMachineSetList(client)
	if err != nil {
		return fmt.Errorf("error calling getMachineSetList: %v", err)
	}
	for key := range machineSetList.Items {
		glog.Infof("MachineSet %q replicas %d. Ready: %d, available %d", machineSetList.Items[key].Name, *machineSetList.Items[key].Spec.Replicas, machineSetList.Items[key].Status.ReadyReplicas, machineSetList.Items[key].Status.AvailableReplicas)
	}
	return nil
}
