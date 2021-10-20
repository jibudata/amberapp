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

package quiesce

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/jibudata/app-hook-operator/api/v1alpha1"
	"github.com/jibudata/app-hook-operator/pkg/client"
	"github.com/jibudata/app-hook-operator/pkg/cmd"
	"github.com/jibudata/app-hook-operator/pkg/util"
)

type QuiesceOptions struct {
	Name string
	//Database    string
}

func NewCommand(client *client.Client) *cobra.Command {

	option := &QuiesceOptions{}

	c := &cobra.Command{
		Use:   "quiesce",
		Short: "Quiesce a Database",
		Long:  "Quiesce a Database",
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(option.Validate(c, client))
			cmd.CheckError(option.Run(client))
		},
	}

	option.BindFlags(c.Flags(), c)

	return c
}

func (q *QuiesceOptions) BindFlags(flags *pflag.FlagSet, c *cobra.Command) {
	flags.StringVarP(&q.Name, "name", "n", "", "database configration name")
	c.MarkFlagRequired("name")
	//flags.StringVarP(&c.Database, "database", "d", "", "name of the database instance")
}

func (q *QuiesceOptions) Validate(command *cobra.Command, kubeclient *client.Client) error {
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

func (q *QuiesceOptions) updateHookCR(kubeclient *client.Client, namespace string) error {
	crName := q.Name + "-hook"

	foundHook := &v1alpha1.AppHook{}
	err := kubeclient.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: namespace,
			Name:      crName,
		},
		foundHook)

	if err != nil {
		return err
	}

	switch foundHook.Status.Phase {
	// Valid states
	case v1alpha1.HookReady:
	case v1alpha1.HookUNQUIESCED:
	// Invalid
	case v1alpha1.HookNotReady:
		return fmt.Errorf("hook CR %s not ready yet", foundHook.Name)
	case v1alpha1.HookQUIESCED:
		return fmt.Errorf("hook CR %s already quiesced", foundHook.Name)
	case v1alpha1.HookQUIESCEINPROGRESS:
		return fmt.Errorf("hook CR %s quiesce already in progress", foundHook.Name)
	case v1alpha1.HookUNQUIESCEINPROGRESS:
		return fmt.Errorf("hook CR %s unquiesce is in progress", foundHook.Name)
	}

	foundHook.Spec.OperationType = v1alpha1.QUIESCE

	return kubeclient.Update(context.TODO(), foundHook)
}

func (q *QuiesceOptions) Run(kubeclient *client.Client) error {
	crName := q.Name + "-hook"
	namespace, err := util.GetOperatorNamespace()
	if err != nil {
		return err
	}

	err = q.updateHookCR(kubeclient, namespace)
	if err == nil {
		fmt.Printf("Update hook success: %s, namespace: %s\n", crName, namespace)
	}

	return err
}