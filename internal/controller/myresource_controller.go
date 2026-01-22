/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"slices"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mygroupv1 "github.com/sapcc/myresource-operator/api/v1"
)

type MyResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

/*
	apiVersion: mygroup.example.com/v1
	kind: MyResource
	metadata:
		name: my-request-resource
		namespace: default
	spec:
		replicas: 3
		image: my-app-image:latest
*/

// Q1: Explain what this controller does when the above MyResource CR is created.
// Q2: What changes would you make to the Reconcile function to handle updates to the MyResource spec?
// Q3: If the Deployment creation fails, how does the controller handle the error?
// Q4: How often will the Reconcile function be requeued?
// Q5: How could you filter reconciled MyResource resources to only those MyResource CRs in a specific namespace?
// Q6: Would you consider different update strategy for the Deployment if the MyResource spec changes? Why or why not?
// Q7: How would you extend this controller to manage additional resources, such as ConfigMaps?
// Q8: What will happen if you delete a MyResource CR? How does kubernetes ensure cleanup of related resources?
// Q9: How does the finalizer mechanism work in this controller, and what is its purpose?
// Q10: What are potential benefits of setting MyResource status?
// Q11: Explain setting availableReplicas in the status field of MyResource.
// Q12: How would you implement unit tests for this Reconcile function to ensure its correctness?

// +kubebuilder:rbac:groups=mygroup.example.com,resources=myresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mygroup.example.com,resources=myresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mygroup.example.com,resources=myresources/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

func (r *MyResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	myResource := &mygroupv1.MyResource{}
	if err := r.Get(ctx, req.NamespacedName, myResource); err != nil {
		log.Error(err, "Failed to fetch MyResource")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if myResource.ObjectMeta.DeletionTimestamp.IsZero() {
		if !slices.Contains(myResource.ObjectMeta.Finalizers, "mygroup.example.com/finalizer") {
			myResource.ObjectMeta.Finalizers = append(myResource.ObjectMeta.Finalizers, "mygroup.example.com/finalizer")
			if err := r.Update(ctx, myResource); err != nil {
				log.Error(err, "Failed to update MyResource with finalizer")
				return ctrl.Result{}, err
			}
		}
	} else {
		if slices.Contains(myResource.ObjectMeta.Finalizers, "mygroup.example.com/finalizer") {
			index := slices.Index(myResource.ObjectMeta.Finalizers, "mygroup.example.com/finalizer")
			myResource.ObjectMeta.Finalizers = slices.Delete(myResource.ObjectMeta.Finalizers, index, index+1)

			if err := r.Update(ctx, myResource); err != nil {
				log.Error(err, "Failed to remove finalizer from MyResource")
				return ctrl.Result{}, err
			}
		}
	}

	deploymentName := myResource.Name + "-deployment"
	deployment := appsv1.Deployment{}

	err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: myResource.Namespace}, &deployment)
	if err == nil {
		log.Info("Deployment already exists, skipping creation", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		r.updateStatus(ctx, myResource, mygroupv1.Error)
		return ctrl.Result{}, nil
	}

	newDeployment := r.createDeployment(myResource)

	if err := ctrl.SetControllerReference(myResource, newDeployment, r.Scheme); err != nil {
		log.Error(err, "Failed to set controller reference")
		r.updateStatus(ctx, myResource, mygroupv1.Error)
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, newDeployment); err != nil {
		log.Error(err, "Failed to create Deployment")
		r.updateStatus(ctx, myResource, mygroupv1.Error)
		return ctrl.Result{}, err
	}

	log.Info("Deployment created successfully", "Deployment.Namespace", newDeployment.Namespace, "Deployment.Name", newDeployment.Name)

	r.updateStatus(ctx, myResource, mygroupv1.Ready)

	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}
func (r *MyResourceReconciler) createDeployment(myResource *mygroupv1.MyResource) *appsv1.Deployment {
	labels := map[string]string{"app": myResource.Name}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      myResource.Name + "-deployment",
			Namespace: myResource.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(myResource.Spec.Replicas)),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "my-app",
							Image: myResource.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *MyResourceReconciler) updateStatus(ctx context.Context, myResource *mygroupv1.MyResource, state mygroupv1.State) error {
	log := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: myResource.Name + "-deployment", Namespace: myResource.Namespace}, deployment)
	if err != nil {
		log.Error(err, "Failed to fetch Deployment for status update")
		return err
	}

	availableReplicas := int(deployment.Status.AvailableReplicas)

	myResource.Status.AvailableReplicas = availableReplicas
	myResource.Status.State = state

	if err := r.Status().Update(ctx, myResource); err != nil {
		log.Error(err, "Failed to update MyResource status")
		return err
	}

	log.Info("MyResource status updated", "AvailableReplicas", availableReplicas)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MyResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mygroupv1.MyResource{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
