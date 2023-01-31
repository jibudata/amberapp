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
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/pkg/client"
	"github.com/jibudata/amberapp/pkg/cmd"
	"github.com/jibudata/amberapp/pkg/util"
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
			cmd.CheckError(option.Validate(c, client))
			cmd.CheckError(option.Run(client))
		},
	}

	option.BindFlags(c.Flags(), c)

	return c
}

func (c *CreateOptions) BindFlags(flags *pflag.FlagSet, command *cobra.Command) {
	flags.StringVarP(&c.Name, "name", "n", "", "database configration name")
	_ = command.MarkFlagRequired("name")
	flags.StringVarP(&c.Provider, "app-provider", "a", "", "database provider, e.g., MySQL")
	_ = command.MarkFlagRequired("app-provider")
	flags.StringVarP(&c.Endpoint, "endpoint", "e", "", "database endpoint, e.g., 'service.namespace', or 'ip:port'")
	_ = command.MarkFlagRequired("endpoint")
	flags.StringArrayVar(&c.Databases, "databases", nil, "databases created inside the DB")
	_ = command.MarkFlagRequired("databases")
	flags.StringVarP(&c.UserName, "username", "u", "", "username of the DB")
	_ = command.MarkFlagRequired("username")
	flags.StringVarP(&c.Password, "password", "p", "", "password for the DB user")
	_ = command.MarkFlagRequired("password")
}

func (c *CreateOptions) Validate(command *cobra.Command, kubeclient *client.Client) error {
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

func (c *CreateOptions) createSecret(kubeclient *client.Client, secretName, namespace string) error {

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

	err := kubeclient.Create(context.TODO(), secret)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			fmt.Printf("secret already exists: %s, namespace: %s\n", c.Name, namespace)
			return nil
		}
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

	return err
}

func (c *CreateOptions) waitUntilReady(kubeclient *client.Client, namespace string) (error, bool) {
	crName := c.Name + "-hook"
	done := false

	err := wait.PollImmediate(DefaultPollInterval, DefaultPollTimeout, func() (bool, error) {
		foundHook := &v1alpha1.AppHook{}
		err := kubeclient.Get(
			context.TODO(),
			types.NamespacedName{
				Namespace: namespace,
				Name:      crName,
			},
			foundHook)

		if err != nil {
			return false, err
		}
		if foundHook.Status.Phase == v1alpha1.HookReady {
			done = true
			return true, nil
		}
		return false, nil
	})

	return err, done
}

func (c *CreateOptions) createApphookCR(kubeclient *client.Client, secretName, namespace string) error {

	hookCR := &v1alpha1.AppHook{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "AppHook",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.Name + "-hook",
			Namespace: namespace,
		},
		Spec: v1alpha1.AppHookSpec{
			Name:        c.Name,
			AppProvider: c.Provider,
			EndPoint:    c.Endpoint,
			Databases:   c.Databases,
			Secret: corev1.SecretReference{
				Name:      secretName,
				Namespace: namespace,
			},
		},
	}

	err := kubeclient.Create(context.TODO(), hookCR)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			fmt.Printf("apphook already exists: %s, namespace: %s\n", c.Name, namespace)
			return nil
		}
		return err
	}
	return nil
}

func (c *CreateOptions) Run(kubeclient *client.Client) error {
	secretName := c.Name + "-token"
	crName := c.Name + "-hook"
	namespace, err := util.GetOperatorNamespace()
	if err != nil {
		return err
	}

	fmt.Printf("Create secret from username and password, secret name: %s, namespace: %s\n", secretName, namespace)
	err = c.createSecret(kubeclient, secretName, namespace)
	if err != nil {
		return err
	}

	fmt.Printf("Create apphook: %s, namespace: %s\n", c.Name, namespace)
	err = c.createApphookCR(kubeclient, secretName, namespace)
	if err != nil {
		return err
	}

	//fmt.Printf("Created apphook: %s, use `kubectl get apphook -n %s %s` to look at the status\n", crName, namespace, crName)
	fmt.Printf("Waiting for db get ready: %s, namespace: %s\n", crName, namespace)
	err, done := c.waitUntilReady(kubeclient, namespace)
	if err != nil {
		fmt.Printf("wait for hook ready error: %s, namespace: %s\n", crName, namespace)
		return err
	}
	if done {
		fmt.Printf("Database is successfully connected and ready: %s, namespace: %s\n", crName, namespace)
	}

	return err
}
