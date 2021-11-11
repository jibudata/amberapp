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
	"strings"

	"github.com/jibudata/amberapp/controllers/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	storagev1alpha1 "github.com/jibudata/amberapp/api/v1alpha1"
	drivermanager "github.com/jibudata/amberapp/controllers/driver"
)

const (
	HasStopWatchAnnotation = "apphooks.storage.jibudata.com/stop-watch"
)

// AppHookReconciler reconciles a AppHook object
type AppHookReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	AppMap map[string]*drivermanager.DriverManager
}

//+kubebuilder:rbac:groups=storage.jibudata.com,resources=apphooks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=storage.jibudata.com,resources=apphooks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=storage.jibudata.com,resources=apphooks/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

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

			// remove app hook
			err = r.ensureRemoveHook(instance)
			if err != nil {
				message := fmt.Sprintf("failed to delete app hook: %s", instance.Name)
				log.Log.Error(err, message)
				return reconcile.Result{}, err
			}
		}

		return reconcile.Result{}, nil
	}

	// database quiesce/unquiesce
	if err = r.ensureHookOperation(instance); err != nil {
		message := fmt.Sprintf("failed to take action of: %s", instance.Name)
		log.Log.Error(err, message)

		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppHookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	predicateFunc := func(obj runtime.Object) bool {
		instance, ok := obj.(*storagev1alpha1.AppHook)
		if !ok {
			return false
		}

		// ignore if Annotation is present
		if _, ok = instance.ObjectMeta.Annotations[HasStopWatchAnnotation]; ok {
			return false
		}

		return true
	}

	appHookCRPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return predicateFunc(e.Object)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return predicateFunc(e.ObjectNew)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return predicateFunc(e.Object)
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&storagev1alpha1.AppHook{}, builder.WithPredicates(appHookCRPredicate)).
		Complete(r)
}

func (r *AppHookReconciler) ensureHookOperation(instance *storagev1alpha1.AppHook) error {
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

	// get drivermanager for the CR
	mgr, err := r.getDriverManager(instance, appSecret)
	if err != nil {
		log.Log.Error(err, fmt.Sprintf("failed to get driver manager for %s", instance.Name))
		return err
	}
	// check operation type
	if instance.Spec.OperationType == "" { // new CR
		if instance.Status.Phase != storagev1alpha1.HookReady {
			instance.Status.Phase = storagev1alpha1.HookCreated
			// connect to database to check status
			err = mgr.DBConnect()
			if err != nil {
				log.Log.Error(err, fmt.Sprintf("failed to connect database for %s", instance.Name))
				instance.Status.Phase = storagev1alpha1.HookNotReady
			} else {
				log.Log.Info(fmt.Sprintf("hook for %s is ready", instance.Name))
				instance.Status.Phase = storagev1alpha1.HookReady
			}
		}
	} else if strings.EqualFold(instance.Spec.OperationType, storagev1alpha1.QUIESCE) {
		if instance.Status.Phase != storagev1alpha1.HookQUIESCED {
			// quiesce database
			log.Log.Info(fmt.Sprintf("quiesce for %s in progress", instance.Name))
			err = mgr.DBQuiesce()
			if err != nil {
				log.Log.Error(err, fmt.Sprintf("failed to quiesce database for %s", instance.Name))
				instance.Status.Phase = storagev1alpha1.HookQUIESCEINPROGRESS
			} else {
				log.Log.Info(fmt.Sprintf("successfully quiesce for %s", instance.Name))
				instance.Status.Phase = storagev1alpha1.HookQUIESCED
			}
		}
	} else if strings.EqualFold(instance.Spec.OperationType, storagev1alpha1.UNQUIESCE) {
		if instance.Status.Phase != storagev1alpha1.HookUNQUIESCED {
			// unquiesce database
			log.Log.Info(fmt.Sprintf("unquiesce for %s in progress", instance.Name))
			err = mgr.DBUnquiesce()
			if err != nil {
				log.Log.Error(err, fmt.Sprintf("failed to unquiesce database for %s", instance.Name))
				instance.Status.Phase = storagev1alpha1.HookUNQUIESCEINPROGRESS
			} else {
				log.Log.Info(fmt.Sprintf("successfully unquiesce for %s", instance.Name))
				instance.Status.Phase = storagev1alpha1.HookUNQUIESCED
			}
		}
	} else {
		log.Log.Error(fmt.Errorf("unsupported operation %s for %s", instance.Spec.OperationType, instance.Name), "err")
	}

	// update CR status
	statusError := r.Client.Status().Update(context.TODO(), instance)
	if statusError != nil {
		log.Log.Error(statusError, fmt.Sprintf("Failed to update status of %s", instance.Name))
		return statusError
	}

	return nil
}

func (r *AppHookReconciler) ensureRemoveHook(instance *storagev1alpha1.AppHook) error {
	return r.deleteDriverManager(instance)
}

func (r *AppHookReconciler) getDriverManager(instance *storagev1alpha1.AppHook, appSecret *corev1.Secret) (*drivermanager.DriverManager, error) {
	// lookup map
	if r.AppMap[instance.Name] == nil {
		// if not exist, create new drivermanager
		mgr, err := drivermanager.NewManager(r.Client, instance, appSecret)
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("Initialize mamager failed for %s", instance.Name))
			return nil, err
		}
		// cache the drivermanager
		r.AppMap[instance.Name] = mgr
	}

	return r.AppMap[instance.Name], nil
}

func (r *AppHookReconciler) deleteDriverManager(instance *storagev1alpha1.AppHook) error {
	// lookup map
	if r.AppMap[instance.Name] != nil {
		// if exist, delete drivermanager
		delete(r.AppMap, instance.Name)
	}

	return nil
}
