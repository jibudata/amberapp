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

package apphook

import (
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/pkg/client"
	"github.com/jibudata/amberapp/pkg/cmd/create"
	"github.com/jibudata/amberapp/pkg/cmd/delete"
	"github.com/jibudata/amberapp/pkg/cmd/quiesce"
	"github.com/jibudata/amberapp/pkg/cmd/unquiesce"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

func NewCommand(baseName string) (*cobra.Command, error) {

	kubeconfig, err := client.NewConfig()
	if err != nil {
		return nil, err
	}
	kubeclient, err := client.NewClient(kubeconfig, scheme)
	if err != nil {
		return nil, err
	}

	cmd := &cobra.Command{}
	cmd.AddCommand(
		create.NewCommand(kubeclient),
		delete.NewCommand(kubeclient),
		quiesce.NewCommand(kubeclient),
		unquiesce.NewCommand(kubeclient),
	)
	return cmd, nil
}
