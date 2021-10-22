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

package client

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	client.Client
	*rest.Config
}

// Config returns a *rest.Config, using in-cluster configuration.
func NewConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})

	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error finding Kubernetes API server config")
	}

	return clientConfig, nil
}

func NewClient(restConfig *rest.Config, scheme *runtime.Scheme) (*Client, error) {
	client, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	kubeclient := &Client{
		Client: client,
		Config: restConfig,
	}

	return kubeclient, nil
}
