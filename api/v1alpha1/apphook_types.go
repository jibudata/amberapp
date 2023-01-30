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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Params key
const (
	QuiesceFromPrimary = "QuiesceFromPrimary"
)

const (
	// mysql param
	MysqlLockMethod   = "mysql-lock-method"
	MysqlTableLock    = "table"
	MysqlInstanceLock = "instance"

	// redis param
	RedisBackupMethod = "redis-backup-method"
	RedisBackupByRDB  = "rdb"
	RedisBackupByAOF  = "aof"
)

// AppHookSpec defines the desired state of AppHook
type AppHookSpec struct {
	// Name is a job for backup/restore/migration
	Name string `json:"name"`
	// AppProvider is the application identifier for different vendors, such as mysql
	AppProvider string `json:"appProvider,omitempty"`
	// Endpoint to connect the applicatio service
	EndPoint string `json:"endPoint,omitempty"`
	// Databases
	Databases []string `json:"databases,omitempty"`
	// OperationType is the operation executed in application
	//+kubebuilder:validation:Enum=quiesce;unquiesce
	OperationType string `json:"operationType,omitempty"`
	// TimeoutSeconds is the timeout of operation
	//+kubebuilder:validation:Minimum=0
	//+kubebuilder:default:0
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
	// Secret to access the application
	Secret corev1.SecretReference `json:"secret,omitempty"`
	// Other options
	Params map[string]string `json:"params,omitempty"`
}

type QuiesceResult struct {
	Mongo *MongoResult `json:"mongo,omitempty"`
	Mysql *MysqlResult `json:"mysql,omitempty"`
	Pg    *PgResult    `json:"pg,omitempty"`
	Redis *RedisResult `json:"redis,omitempty"`
}

type MongoResult struct {
	MongoEndpoint string `json:"mongoEndpoint,omitempty"`
	IsPrimary     bool   `json:"isPrimary,omitempty"`
}

type MysqlResult struct {
}

type PgResult struct {
}

type RedisResult struct {
}

// PreservedConfig saves the origin params before change by quiesce
type PreservedConfig struct {
	Params map[string]string `json:"params,omitempty"`
}

// AppHookStatus defines the observed state of AppHook
// +kubebuilder:subresource:status
type AppHookStatus struct {
	Phase             string           `json:"phase,omitempty"`
	ErrMsg            string           `json:"errMsg,omitempty"`
	QuiescedTimestamp *metav1.Time     `json:"quiescedTimestamp,omitempty"`
	Result            *QuiesceResult   `json:"result,omitempty"`
	PreservedConfig   *PreservedConfig `json:"preservedConfig,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=.metadata.creationTimestamp
//+kubebuilder:printcolumn:name="Created At",type=string,JSONPath=.metadata.creationTimestamp
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=.status.phase,description="Phase"

// AppHook is the Schema for the apphooks API
type AppHook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppHookSpec   `json:"spec,omitempty"`
	Status AppHookStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AppHookList contains a list of AppHook
type AppHookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AppHook `json:"items"`
}

// operation type
const (
	// quiesce operation
	QUIESCE = "quiesce"
	// unquiesce operation
	UNQUIESCE = "unquiesce"
)

// phase
const (
	HookCreated             = "Created"
	HookReady               = "Ready"
	HookNotReady            = "NotReady"
	HookQUIESCEINPROGRESS   = "Quiesce In Progress"
	HookQUIESCED            = "Quiesced"
	HookUNQUIESCEINPROGRESS = "Unquiesce In Progress"
	HookUNQUIESCED          = "Unquiesced"
)

func init() {
	SchemeBuilder.Register(&AppHook{}, &AppHookList{})
}
