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

package create

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/jibudata/app-hook-operator/pkg/client"
	"github.com/jibudata/app-hook-operator/pkg/cmd"
	"github.com/jibudata/app-hook-operator/pkg/util"
)

const (
	UserNameKey = "username"
	PasswordKey = "password"

	DefaultPollInterval = 1 * time.Second
	DefaultPollTimeout  = 30 * time.Second
)

type CreateOptions struct {
	Name      string
	Provider  string
	Endpoint  string
	Databases []string
	UserName  string
	Password  string
}

func NewCommand(client *client.Client) *cobra.Command {

	option := &CreateOptions{}

	c := &cobra.Command{
		Use:   "create",
		Short: "Create a Database configuration",
		Long:  "Create a Database configraution which will be used for quiesce/resume operations",
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(option.Run(client))
		},
	}

	option.BindFlags(c.Flags())

	return c
}

func (c *CreateOptions) BindFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&c.Name, "name", "n", "", "database configration name")
	flags.StringVar(&c.Provider, "provider", "", "database provider, e.g., MySQL")
	flags.StringVarP(&c.Endpoint, "endpoint", "e", "", "database endpoint, e.g., 'service.namespace', or 'ip:port'")
	flags.StringArrayVar(&c.Databases, "databases", nil, "databases created inside the DB")
	flags.StringVarP(&c.UserName, "username", "u", "", "username of the DB")
	flags.StringVarP(&c.Password, "password", "p", "", "password for the DB user")
}

func (c *CreateOptions) createSecret(kubeclient *client.Client) error {

	secretName := c.Name + "-token"
	namespace, err := util.GetOperatorNamespace()
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			UserNameKey: []byte(c.UserName),
			PasswordKey: []byte(c.Password),
		},
	}

	err = kubeclient.Create(context.TODO(), secret)
	if err != nil {
		return err
	}

	err = wait.PollImmediate(DefaultPollInterval, DefaultPollTimeout, func() (bool, error) {
		foundSecret := &corev1.Secret{}
		err := kubeclient.Get(
			context.TODO(),
			types.NamespacedName{
				Namespace: namespace,
				Name:      secretName,
			},
			foundSecret)

		if err != nil {
			return false, err
		}
		return true, nil
	})

	return nil
}

func (c *CreateOptions) Run(kubeclient *client.Client) error {
	return nil
}
