/*
Copyright 2024.

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
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mydomainv1 "github.com/yleoer/webserver-operator/api/v1"
)

// WebServerReconciler reconciles a WebServer object
type WebServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=my.domain,resources=webservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=my.domain,resources=webservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=my.domain,resources=webservers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the WebServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *WebServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx).WithValues("WebServer", req.NamespacedName)

	instance := &mydomainv1.WebServer{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Return and don't requeue
			l.Info("Webserver resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		l.Error(err, "Failed to get WebServer")
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// Check if the webserver deployment already exists, if not, create a new one
	found := &v1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		dep := r.deploymentForWebserver(instance)
		l.Info("Creating a new Deployment", "Namespace", dep.Namespace, "Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			l.Error(err, "Failed to create new deployment", "Namespace", dep.Namespace, "Name", dep.Name)
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

		// Deployment created successfully
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		l.Error(err, "Failed to get Deployment")
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// Ensure the deployment replicas and image are the same as the spec
	var replicas int32 = int32(instance.Spec.Replicas)
	image := instance.Spec.Image

	var needUpd bool
	if oldReplicas := *found.Spec.Replicas; oldReplicas != replicas {
		l.Info("Deployment spec.replicas change", "from", *found.Spec.Replicas, "to", replicas)
		found.Spec.Replicas = &replicas
		needUpd = true
	}

	if oldImage := (*found).Spec.Template.Spec.Containers[0].Image; oldImage != image {
		l.Info("Deployment spec.template.spec.container[0].image change", "from", oldImage, "to", image)
		found.Spec.Template.Spec.Containers[0].Image = image
		needUpd = true
	}

	if needUpd {
		if err = r.Update(ctx, found); err != nil {
			l.Error(err, "Failed to update Deployment", "Namespace", found.Namespace, "Name", found.Name)
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

		// Spec updated
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if the webserver service already exists, if not, create a new one
	foundService := &v12.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: instance.Name + "-service", Namespace: instance.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		// Define a new service
		srv := r.serviceForWebserver(instance)
		l.Info("Creating a new Service", "Namespace", srv.Namespace, "Name", srv.Name)
		if err = r.Create(ctx, srv); err != nil {
			l.Error(err, "Failed to create new Service", "Namespace", srv.Namespace, "Name", srv.Name)
			return ctrl.Result{RequeueAfter: time.Second * 5}, err
		}

		// Service created successfully
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		l.Error(err, "Failed to get Service", "Namespace", foundService.Namespace, "Name", foundService.Name)
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// Tbd: Ensure the service state is the same as the spec, your homework

	// reconcile webserver operator in again 10 seconds

	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mydomainv1.WebServer{}).
		Complete(r)
}

// deploymentForWebserver returns a webserver Deployment object
func (r *WebServerReconciler) deploymentForWebserver(ws *mydomainv1.WebServer) *v1.Deployment {
	labels := labelsForWebserver(ws.Name)
	var replicas int32 = int32(ws.Spec.Replicas)

	dep := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ws.Name,
			Namespace: ws.Namespace,
		},
		Spec: v1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: v12.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},

				Spec: v12.PodSpec{
					Containers: []v12.Container{{
						Image: ws.Spec.Image,
						Name:  "webserver",
					}},
				},
			},
		},
	}
	// Set Webserver instance as the owner and controller
	ctrl.SetControllerReference(ws, dep, r.Scheme)
	return dep
}

// serviceForWebserver returns a webserver-service service object
func (r *WebServerReconciler) serviceForWebserver(ws *mydomainv1.WebServer) *v12.Service {
	labels := labelsForWebserver(ws.Name)
	srv := &v12.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ws.Name + "-service",
			Namespace: ws.Namespace,
			Labels:    labels,
		},
		Spec: v12.ServiceSpec{
			Type: v12.ServiceTypeNodePort,
			Ports: []v12.ServicePort{
				v12.ServicePort{
					Protocol: v12.ProtocolTCP,
					NodePort: 30010,
					Port:     80,
				},
			},
			Selector: map[string]string{
				"app":          "webserver",
				"webserver_cr": ws.Name,
			},
		},
	}

	// Set Webserver instance as the owner and controller
	ctrl.SetControllerReference(ws, srv, r.Scheme)
	return srv
}

// labelsForWebserver returns the labels for selecting the resources
// belonging to the given Webserver CR name.
func labelsForWebserver(name string) map[string]string {
	return map[string]string{"app": "webserver", "webserver_cr": name}
}
