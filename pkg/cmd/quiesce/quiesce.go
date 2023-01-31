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
	DefaultPollTimeout  = 60 * time.Second
)

type QuiesceOptions struct {
	Name string
	Wait bool
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
	_ = c.MarkFlagRequired("name")
	flags.BoolVarP(&q.Wait, "wait", "w", false, "wait for quiescd")
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

func (q *QuiesceOptions) waitUntilQuiesced(kubeclient *client.Client, namespace string) (error, bool) {
	crName := q.Name + "-hook"
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
		if foundHook.Status.Phase == v1alpha1.HookQUIESCED {
			done = true
			return true, nil
		}
		return false, nil
	})

	return err, done
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
	} else {
		return err
	}

	if q.Wait {
		startTime := time.Now()
		fmt.Printf("Waiting for db get quiesced: %s, namespace: %s\n", crName, namespace)
		err, done := q.waitUntilQuiesced(kubeclient, namespace)
		doneTime := time.Now()
		duration := doneTime.Sub(startTime)
		if err != nil {
			fmt.Printf("wait for hook into quiesced state error: %s, namespace: %s\n", crName, namespace)
			return err
		}
		if done {
			fmt.Printf("Database is successfully quiesced: %s, namespace: %s, duration: %s\n", crName, namespace, duration)
		}
	}

	return nil
}
