/*
Copyright 2021.

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

package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/jibudata/app-hook-operator/controllers/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	storagev1alpha1 "github.com/jibudata/app-hook-operator/api/v1alpha1"
)

// getWatchNamespace returns the Namespace the operator should be watching for changes
func getOperatorNamespace() (string, error) {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}
	return ns, nil
}

// AppHookReconciler reconciles a AppHook object
type AppHookReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=storage.jibudata.com,resources=apphooks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=storage.jibudata.com,resources=apphooks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=storage.jibudata.com,resources=apphooks/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AppHook object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *AppHookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// your logic here
	instance := &storagev1alpha1.AppHook{}
	if getInstanceErr := r.Get(ctx, req.NamespacedName, instance); getInstanceErr != nil {
		if errors.IsNotFound(getInstanceErr) {
			log.Log.Info("No apphook instance found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, getInstanceErr
	}

	result, err := r.reconcile(instance)

	if err != nil {
		log.Log.Error(err, fmt.Sprintf("Failed to reconcile %s", instance.Name))
		return result, err
	} else {
		return result, nil
	}
}

func (r *AppHookReconciler) reconcile(instance *storagev1alpha1.AppHook) (ctrl.Result, error) {
	var err error
	var apphookFinalizer = "apphook.storage.jibudata.com"

	// Check GetDeletionTimestamp to determine if the object is under deletion
	if instance.GetDeletionTimestamp().IsZero() {
		if !util.IsContain(instance.GetFinalizers(), apphookFinalizer) {
			log.Log.Info(fmt.Sprintf("Append apphook to finalizer %s", instance.Name))
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, apphookFinalizer)
			if err := r.Client.Update(context.TODO(), instance); err != nil {
				log.Log.Error(err, fmt.Sprintf("Failed to update AppHook with finalizer %s", instance.Name))
				return reconcile.Result{}, err
			}
		}
	} else {
		// The object is marked for deletion
		if util.IsContain(instance.GetFinalizers(), apphookFinalizer) {
			log.Log.Info(fmt.Sprintf("Removing finalizer from %s", instance.Name))

			// Once all finalizers have been removed, the object will be deleted
			instance.ObjectMeta.Finalizers = util.Remove(instance.ObjectMeta.Finalizers, apphookFinalizer)
			if err := r.Client.Update(context.TODO(), instance); err != nil {
				log.Log.Error(err, fmt.Sprintf("Failed to remove finalizer from %s", instance.Name))
				return reconcile.Result{}, err
			}

			// uninstall
			err = r.uninstallHookDeployment(instance)
			if err != nil {
				message := fmt.Sprintf("failed to delete HookDeployment: %s", instance.Name)
				log.Log.Error(err, message)
				return reconcile.Result{}, err
			}
		}

		return reconcile.Result{}, nil
	}

	// install
	log.Log.Info("step: ensureHookDeployment")
	if err = r.ensureHookDeployment(instance); err != nil {
		message := fmt.Sprintf("failed to ensureHookDeployment: %s", instance.Name)
		log.Log.Error(err, message)

		return reconcile.Result{}, err
	}

	instance.Status.Phase = storagev1alpha1.HookCreated
	statusError := r.Client.Status().Update(context.TODO(), instance)
	if statusError != nil {
		log.Log.Error(statusError, fmt.Sprintf("Failed to update status of %s", instance.Name))
		return reconcile.Result{}, statusError
	}

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppHookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&storagev1alpha1.AppHook{}).
		Complete(r)
}

func (r *AppHookReconciler) ensureHookDeployment(instance *storagev1alpha1.AppHook) error {

	deploymentName := instance.Name

	// check secret in the same namespace with operator
	operatorNS, _ := getOperatorNamespace()
	if instance.Spec.Secret.Namespace != "" {
		if operatorNS != instance.Spec.Secret.Namespace {
			nsErr := fmt.Errorf("secret %s namespace %s is different with operator namespace %s", instance.Spec.Secret.Name, instance.Spec.Secret.Namespace, operatorNS)
			log.Log.Error(nsErr, "")
			return nsErr
		}
	} else {
		instance.Spec.Secret.Namespace = operatorNS
	}

	appSecret := &corev1.Secret{}
	err := r.Client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      instance.Spec.Secret.Name,
			Namespace: instance.Spec.Secret.Namespace,
		}, appSecret)

	if err != nil {
		log.Log.Error(err, fmt.Sprintf("failed to get secret %s in namespace %s", instance.Spec.Secret.Name, instance.Spec.Secret.Namespace))
		return err
	}

	expectedDeployment, err := InitHookDeployment(instance, appSecret)
	if err != nil {
		log.Log.Error(err, fmt.Sprintf("failed to init deployment %s in namespace %s", instance.Name, instance.Namespace))
		return err
	}

	foundDeployment := &appsv1.Deployment{}
	err = r.Client.Get(
		context.TODO(),
		types.NamespacedName{Name: deploymentName, Namespace: instance.Namespace},
		foundDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Log.Info(fmt.Sprintf("create Hook deployment %s", instance.Name))
			return r.Client.Create(context.TODO(), expectedDeployment)
		}

		log.Log.Error(err, fmt.Sprintf("failed to create Hook deployment %s", instance.Name))
		return err
	}

	if reflect.DeepEqual(foundDeployment.Spec, expectedDeployment.Spec) {
		return nil
	}

	updatedDeployment := updateHookDeployment(foundDeployment, expectedDeployment)
	if updatedDeployment != nil {
		log.Log.Info(fmt.Sprintf("update Hook deployment %s", instance.Name))
		return r.Client.Update(context.TODO(), updatedDeployment)
	}

	return nil
}

func (r *AppHookReconciler) uninstallHookDeployment(instance *storagev1alpha1.AppHook) error {
	deploymentName := instance.Name

	foundDeployment := &appsv1.Deployment{}
	err := r.Client.Get(
		context.TODO(),
		types.NamespacedName{Name: deploymentName, Namespace: instance.Namespace},
		foundDeployment)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Log.Info(fmt.Sprintf("appHook deployment %s is already deleted", instance.Name))
			return nil
		}
		log.Log.Error(err, fmt.Sprintf("failed to get Hook deployment %s", instance.Name))
		return err
	}

	err = r.Client.Delete(context.TODO(), foundDeployment)
	if err != nil {
		log.Log.Error(err, fmt.Sprintf("failed to delete Hook deployment %s", instance.Name))
		return err
	}

	return nil
}
