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
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	storagev1alpha1 "github.com/jibudata/amberapp/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CalculateDataHash generates a sha256 hex-digest for a data object
func CalculateDataHash(dataObject interface{}) (string, error) {
	data, err := json.Marshal(dataObject)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func IsContain(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func Remove(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func GetLabels(clusterName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": clusterName,
	}
}

func InitK8sEvent(instance *storagev1alpha1.AppHook, eventtype, reason, message string) *corev1.Event {
	t := metav1.Time{Time: time.Now()}
	selectLabels := GetLabels(instance.Name)
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%v.%x", instance.Name, t.UnixNano()),
			Namespace: instance.Namespace,
			Labels:    selectLabels,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:            instance.Kind,
			Namespace:       instance.Namespace,
			Name:            instance.Name,
			UID:             instance.UID,
			ResourceVersion: instance.ResourceVersion,
			APIVersion:      instance.APIVersion,
		},
		Reason:         reason,
		Message:        message,
		FirstTimestamp: t,
		LastTimestamp:  t,
		Count:          1,
		Type:           eventtype,
	}
}
