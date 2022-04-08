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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/controllers/util"
)

const (
	// Hook related dependencies
	secretUserKey   = "username"
	secretPasswdKey = "password"
)

func InitHookDeployment(
	instance *v1alpha1.AppHook,
	secret *corev1.Secret) (*appsv1.Deployment, error) {

	var replicaOne int32 = 1
	var pullPolicy = corev1.PullIfNotPresent
	var image = "hook:v1"

	deploymentName := instance.Name

	labels := util.GetLabels(instance.Name)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: instance.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: instance.APIVersion,
					Kind:       instance.Kind,
					Name:       instance.Name,
					UID:        instance.UID,
				},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicaOne,
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
							Name:            deploymentName,
							Image:           image,
							ImagePullPolicy: pullPolicy,
							Env: []corev1.EnvVar{
								{
									Name:  "DEPLOYMENTNAME",
									Value: deploymentName,
								},
								{
									Name:  "NAMESPACE",
									Value: instance.Namespace,
								},
								{
									Name: "USERNAME",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secret.Name,
											},
											Key: secretUserKey,
										},
									},
								},
								{
									Name: "PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secret.Name,
											},
											Key: secretPasswdKey,
										},
									},
								},
								{
									Name:  "REST_API_IP",
									Value: instance.Spec.EndPoint,
								},
								{
									Name:  "PROVIDER",
									Value: instance.Spec.AppProvider,
								},
								{
									Name:  "OPERATION",
									Value: instance.Spec.OperationType,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("0.5"),
									corev1.ResourceMemory: resource.MustParse("500Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					ServiceAccountName: "amberapp-controller-manager",
				},
			},
		},
	}, nil
}
