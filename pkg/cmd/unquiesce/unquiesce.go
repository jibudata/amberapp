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

package unquiesce

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/pkg/client"
	"github.com/jibudata/amberapp/pkg/cmd"
	"github.com/jibudata/amberapp/pkg/util"
)

const (
	DefaultPollInterval = 1 * time.Second
	DefaultPollTimeout  = 20 * time.Second
)

type UnquiesceOptions struct {
	Name string
	//Database    string
}

func NewCommand(client *client.Client) *cobra.Command {

	option := &UnquiesceOptions{}

	c := &cobra.Command{
		Use:   "unquiesce",
		Short: "Unquiesce a Database",
		Long:  "Unquiesce a Database which has been quiesced",
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(option.Validate(c, client))
			cmd.CheckError(option.Run(client))
		},
	}

	option.BindFlags(c.Flags(), c)

	return c
}

func (u *UnquiesceOptions) BindFlags(flags *pflag.FlagSet, c *cobra.Command) {
	flags.StringVarP(&u.Name, "name", "n", "", "database configration name")
	_ = c.MarkFlagRequired("name")
	//flags.StringVarP(&c.Database, "database", "d", "", "name of the database instance")
}

func (u *UnquiesceOptions) Validate(command *cobra.Command, kubeclient *client.Client) error {
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

func (u *UnquiesceOptions) updateHookCR(kubeclient *client.Client, namespace string) error {
	crName := u.Name + "-hook"

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
	case v1alpha1.HookReady:
		return fmt.Errorf("hook CR %s not quiesced yet", foundHook.Name)
	case v1alpha1.HookNotReady:
		return fmt.Errorf("hook CR %s not ready yet", foundHook.Name)
	case v1alpha1.HookUNQUIESCED:
		return fmt.Errorf("hook CR %s already unquiesced", foundHook.Name)
	case v1alpha1.HookQUIESCEINPROGRESS:
		return fmt.Errorf("hook CR %s quiesce still in progress, please wait", foundHook.Name)
	case v1alpha1.HookUNQUIESCEINPROGRESS:
		return fmt.Errorf("hook CR %s unquiesce already in progress", foundHook.Name)
	case v1alpha1.HookQUIESCED:
	}

	foundHook.Spec.OperationType = v1alpha1.UNQUIESCE

	return kubeclient.Update(context.TODO(), foundHook)
}

func (u *UnquiesceOptions) waitUntilUnquiesced(kubeclient *client.Client, namespace string) (error, bool) {
	crName := u.Name + "-hook"
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
		if foundHook.Status.Phase == v1alpha1.HookUNQUIESCED {
			done = true
			return true, nil
		}
		return false, nil
	})

	return err, done
}

func (u *UnquiesceOptions) Run(kubeclient *client.Client) error {
	crName := u.Name + "-hook"
	namespace, err := util.GetOperatorNamespace()
	if err != nil {
		return err
	}

	err = u.updateHookCR(kubeclient, namespace)
	if err == nil {
		fmt.Printf("Update hook success: %s, namespace: %s\n", crName, namespace)
	} else {
		return err
	}

	startTime := time.Now()
	err, done := u.waitUntilUnquiesced(kubeclient, namespace)
	doneTime := time.Now()
	duration := doneTime.Sub(startTime)
	if err != nil {
		return err
	}
	if done {
		fmt.Printf("Database is successfully unquiesced: %s, namespace: %s, duration: %s\n", crName, namespace, duration)
	}

	return err
}
