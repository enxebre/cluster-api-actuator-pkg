package operators

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/openshift/cluster-api-actuator-pkg/pkg/framework"
	"k8s.io/utils/pointer"
)

var (
	maoDeployment               = "machine-api-operator"
	maoManagedDeployment        = "machine-api-controllers"
	terminationHandlerDaemonSet = "machine-api-termination-handler"
)
var _ = Describe("[Feature:Operators] Machine API operator deployment should", func() {
	defer GinkgoRecover()

	It("be available", func() {
		client, err := framework.LoadClient()
		Expect(err).NotTo(HaveOccurred())
		Expect(framework.IsDeploymentAvailable(client, maoDeployment, framework.MachineAPINamespace)).To(BeTrue())
	})

	It("reconcile controllers deployment", func() {
		client, err := framework.LoadClient()
		Expect(err).NotTo(HaveOccurred())

		initialDeployment, err := framework.GetDeployment(client, maoManagedDeployment, framework.MachineAPINamespace)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("checking deployment %q is available", maoManagedDeployment))
		Expect(framework.IsDeploymentAvailable(client, maoManagedDeployment, framework.MachineAPINamespace)).To(BeTrue())

		By(fmt.Sprintf("deleting deployment %q", maoManagedDeployment))
		Expect(framework.DeleteDeployment(client, initialDeployment)).NotTo(HaveOccurred())

		By(fmt.Sprintf("checking deployment %q is available again", maoManagedDeployment))
		Expect(framework.IsDeploymentAvailable(client, maoManagedDeployment, framework.MachineAPINamespace)).To(BeTrue())

		By(fmt.Sprintf("checking deployment %q spec matches", maoManagedDeployment))
		Expect(framework.IsDeploymentSynced(client, initialDeployment, maoManagedDeployment, framework.MachineAPINamespace)).To(BeTrue())
	})

	It("maintains deployment spec", func() {
		client, err := framework.LoadClient()
		Expect(err).NotTo(HaveOccurred())

		initialDeployment, err := framework.GetDeployment(client, maoManagedDeployment, framework.MachineAPINamespace)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("checking deployment %q is available", maoManagedDeployment))
		Expect(framework.IsDeploymentAvailable(client, maoManagedDeployment, framework.MachineAPINamespace)).To(BeTrue())

		changedDeployment := initialDeployment.DeepCopy()
		changedDeployment.Spec.Replicas = pointer.Int32Ptr(0)

		By(fmt.Sprintf("updating deployment %q", maoManagedDeployment))
		Expect(framework.UpdateDeployment(client, maoManagedDeployment, framework.MachineAPINamespace, changedDeployment)).NotTo(HaveOccurred())

		By(fmt.Sprintf("checking deployment %q spec matches", maoManagedDeployment))
		Expect(framework.IsDeploymentSynced(client, initialDeployment, maoManagedDeployment, framework.MachineAPINamespace)).To(BeTrue())

		By(fmt.Sprintf("checking deployment %q is available again", maoManagedDeployment))
		Expect(framework.IsDeploymentAvailable(client, maoManagedDeployment, framework.MachineAPINamespace)).To(BeTrue())

	})

	It("reconcile termination handler daemonSet", func() {
		client, err := framework.LoadClient()
		Expect(err).NotTo(HaveOccurred())

		initialDaemonSet, err := framework.GetDaemonSet(client, terminationHandlerDaemonSet, framework.MachineAPINamespace)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("checking daemonSet is available"))
		Expect(framework.IsDaemonSetAvailable(client, terminationHandlerDaemonSet, framework.MachineAPINamespace)).To(BeTrue())

		By(fmt.Sprintf("deleting daemonSet"))
		Expect(framework.DeleteDaemonSet(client, initialDaemonSet)).NotTo(HaveOccurred())

		By(fmt.Sprintf("checking daemonSet is available again"))
		Expect(framework.IsDaemonSetAvailable(client, terminationHandlerDaemonSet, framework.MachineAPINamespace)).To(BeTrue())

		By(fmt.Sprintf("checking got daemonSet spec matches the initial one"))
		Expect(framework.IsDaemonSetSynced(client, initialDaemonSet, terminationHandlerDaemonSet, framework.MachineAPINamespace)).To(BeTrue())

		By(fmt.Sprintf("updating got daemonSet spec"))
		changedDaemonSet := initialDaemonSet.DeepCopy()
		changedDaemonSet.Spec.Template.Spec.NodeSelector = map[string]string{"badSelector": ""}
		Expect(framework.UpdateDaemonSet(client, terminationHandlerDaemonSet, framework.MachineAPINamespace, changedDaemonSet)).NotTo(HaveOccurred())

		By(fmt.Sprintf("checking got daemonSet %q spec matches the initial one again", terminationHandlerDaemonSet))
		Expect(framework.IsDaemonSetSynced(client, initialDaemonSet, terminationHandlerDaemonSet, framework.MachineAPINamespace)).To(BeTrue())
	})
})

var _ = Describe("[Feature:Operators] Machine API cluster operator status should", func() {
	defer GinkgoRecover()

	It("be available", func() {
		client, err := framework.LoadClient()
		Expect(err).NotTo(HaveOccurred())
		Expect(framework.IsStatusAvailable(client, "machine-api")).To(BeTrue())
	})
})
