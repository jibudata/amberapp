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

package delete

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/jibudata/app-hook-operator/api/v1alpha1"
	"github.com/jibudata/app-hook-operator/pkg/client"
	"github.com/jibudata/app-hook-operator/pkg/cmd"
	"github.com/jibudata/app-hook-operator/pkg/util"
)

type DeleteOptions struct {
	Name string
}

func NewCommand(client *client.Client) *cobra.Command {

	option := &DeleteOptions{}

	c := &cobra.Command{
		Use:   "delete",
		Short: "Delete a Database configuration",
		Long:  "Delete a Database configraution",
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(option.Validate(c, client))
			cmd.CheckError(option.Run(client))
		},
	}

	option.BindFlags(c.Flags(), c)

	return c
}

func (d *DeleteOptions) BindFlags(flags *pflag.FlagSet, command *cobra.Command) {
	flags.StringVarP(&d.Name, "name", "n", "", "database configration name")
	command.MarkFlagRequired("name")
}

func (d *DeleteOptions) Validate(command *cobra.Command, kubeclient *client.Client) error {
	// Check WATCH_NAMESPACE, and if namespace exits, apphook operator is running
	namespace, err := util.GetOperatorNamespace()
	if err != nil {
		return err
	}
	ns := &corev1.Namespace{}
	err = kubeclient.Get(
		context.TODO(),
		types.NamespacedName{
			Name: namespace,
		},
		ns)

	if err != nil {
		return err
	}

	return nil
}

func (c *DeleteOptions) deleteSecret(kubeclient *client.Client, secretName, namespace string) error {
	foundSecret := &corev1.Secret{}
	err := kubeclient.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: namespace,
			Name:      secretName,
		},
		foundSecret)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return kubeclient.Delete(context.TODO(), foundSecret)
}

func (d *DeleteOptions) deleteHookCR(kubeclient *client.Client, namespace string) error {
	crName := d.Name + "-hook"

	foundHook := &v1alpha1.AppHook{}
	err := kubeclient.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: namespace,
			Name:      crName,
		},
		foundHook)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return kubeclient.Delete(context.TODO(), foundHook)
}

func (d *DeleteOptions) Run(kubeclient *client.Client) error {
	secretName := d.Name + "-token"
	crName := d.Name + "-hook"
	namespace, err := util.GetOperatorNamespace()
	if err != nil {
		return err
	}

	err = d.deleteSecret(kubeclient, secretName, namespace)
	if err != nil {
		return nil
	}
	fmt.Printf("Delete secret success: %s, namespace: %s\n", secretName, namespace)

	err = d.deleteHookCR(kubeclient, namespace)
	if err == nil {
		fmt.Printf("Delete database configuration success: %s, namespace: %s\n", crName, namespace)
	}

	return err
}
