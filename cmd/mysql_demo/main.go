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

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/pkg/client"
	"github.com/jibudata/amberapp/pkg/cmd"
	"github.com/jibudata/amberapp/pkg/util"
)

const (
	DefaultInterval = 250 * time.Millisecond
)

const (
	StressOperationReplicate = "replicate"
)

const (
	NewTableSuffix = "_backup"
)

var (
	scheme = runtime.NewScheme()
)

type DBConfig struct {
	db       *sql.DB
	Provider string
	Endpoint string
	Database string
	UserName string
	Password string
}

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

	option := &DemoOptions{}

	c := &cobra.Command{
		Use:   "mysqlops",
		Short: "Do some SQL operations",
		Long:  "Do some SQL operations",
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(option.Validate(c, kubeclient))
			cmd.CheckError(option.Run(kubeclient))
		},
	}

	option.BindFlags(c.Flags(), c)

	return c, nil
}

type DemoOptions struct {
	HookName  string
	Database  string
	TableName string
	Operation string
	NumLoops  int
}

func (d *DemoOptions) BindFlags(flags *pflag.FlagSet, c *cobra.Command) {
	flags.StringVarP(&d.HookName, "name", "n", "", "database hook name")
	c.MarkFlagRequired("name")
	flags.StringVarP(&d.Database, "database", "d", "", "name of the database instance")
	c.MarkFlagRequired("database")
	flags.StringVarP(&d.TableName, "table", "t", "employees", "name of the table as data source")
	flags.IntVarP(&d.NumLoops, "count", "c", 10, "number of loops to execute")
	flags.StringVarP(&d.Operation, "operation", "o", "replicate", "supported operation, onyl replicate is supported right now")
}

func (d *DemoOptions) Validate(command *cobra.Command, kubeclient *client.Client) error {
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

func queryAndInsert(db *sql.DB, source, target string) error {
	query := fmt.Sprintf("SELECT * FROM %s", source)
	res, err := db.Query(query)
	defer res.Close()

	if err != nil {
		return nil
	}

	for res.Next() {
		var id int
		var bdate string
		var fname string
		var lname string
		var gender string
		var hdate string
		err := res.Scan(&id, &bdate, &fname, &lname, &gender, &hdate)
		if err != nil {
			klog.InfoS("Failed scan table record", "error", err)
			return err
		}
		//klog.InfoS("get record",
		//	"id", id,
		//	"birth_date", bdate,
		//	"first name", fname,
		//	"last name", lname,
		//	"gender", gender,
		//	"hire date", hdate)

		klog.InfoS("insert record",
			"id", id,
			"birth_date", bdate,
			"first name", fname,
			"last name", lname,
			"gender", gender,
			"hire date", hdate)

		time.Sleep(DefaultInterval)
		sql := fmt.Sprintf("INSERT INTO %s(emp_no, birth_date, first_name, last_name, gender, hire_date) VALUES (?, ?, ?, ?, ?, ?)", target)
		stmt, err := db.Prepare(sql)
		if err != nil {
			klog.InfoS("Failed to prepare query", "error", err)
			return err
		}
		defer stmt.Close()
		res, err := stmt.Exec(id, bdate, fname, lname, gender, hdate)
		if err != nil {
			klog.InfoS("Failed to exec query", "error", err)
			return err
		}

		rows, err := res.RowsAffected()
		if err != nil {
			klog.InfoS("Failed to get rows affected", "error", err)
			return err
		}
		klog.InfoS("insert success", "rows", rows)
	}
	return nil
}

func deleteTable(db *sql.DB, table string) error {
	sql := fmt.Sprintf("DROP TABLE IF EXISTS %s", table)
	_, err := db.Exec(sql)
	if err != nil {
		klog.InfoS("Failed to insert table record", "error", err)
		return err
	}

	return nil
}

func createTable(db *sql.DB, table string) error {
	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s(emp_no int primary key auto_increment, birth_date date, first_name VARCHAR(14), last_name VARCHAR(16), gender ENUM ('M','F'), hire_date DATE)", table)

	ctx, cancelfunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelfunc()
	res, err := db.ExecContext(ctx, query)
	if err != nil {
		klog.InfoS("Failed creating table", "error", err)
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		klog.InfoS("Failed getting rows affected", "error", err)
		return err
	}
	klog.InfoS("Rows affected when creating table", "rows", rows)
	return nil
}

// Create new table;
// Select from source table and insert into new table
// Delete the table, and loop n times
func tableReplicateLoop(config *DBConfig, numLoops int, tableName string) error {
	if len(tableName) == 0 {
		return fmt.Errorf("Empty table name speicified")
	}

	klog.InfoS("table replicate test start", "table", tableName, "loop", numLoops)
	for i := 0; i < numLoops; i++ {
		newTableName := tableName + NewTableSuffix
		err := deleteTable(config.db, newTableName)
		if err != nil {
			return err
		}
		err = createTable(config.db, newTableName)
		if err != nil {
			return err
		}
		err = queryAndInsert(config.db, tableName, newTableName)
		if err != nil {
			return err
		}
		err = deleteTable(config.db, newTableName)
		if err != nil {
			return err
		}
	}
	return nil
}

func dbConnect(config *DBConfig) error {
	var err error
	klog.Info("Connect mysql, endpoint: ", config.Endpoint)

	dsn := fmt.Sprintf("%s:%s@%s(%s)/%s", config.UserName, config.Password, "tcp", config.Endpoint, config.Database)
	config.db, err = sql.Open("mysql", dsn)
	if err != nil {
		klog.InfoS("failed to init connection to mysql database", "database", config.Database, "error", err)
		return err
	}
	err = config.db.Ping()
	if err != nil {
		klog.InfoS("cannot access mysql databases", "database", config.Database)
		return err
	}
	config.db.SetConnMaxLifetime(time.Second * 10)
	//m.db.Close()

	return nil
}

func (d *DemoOptions) getDBSecret(kubeclient *client.Client, name, namespace string) (*corev1.Secret, error) {
	appSecret := &corev1.Secret{}
	err := kubeclient.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, appSecret)

	if err != nil {
		klog.InfoS("Failed to get DB secret", "error", err, "secret", name, "namespace", namespace)
		return nil, err
	}

	return appSecret, err
}

func (d *DemoOptions) dbStress(kubeclient *client.Client, hook *v1alpha1.AppHook) error {
	// Get secret
	dbSecret, err := d.getDBSecret(kubeclient, hook.Spec.Secret.Name, hook.Spec.Secret.Namespace)
	if err != nil {
		return err
	}

	klog.InfoS("Start stress", "user", string(dbSecret.Data["username"]), "password", dbSecret.Data["password"])
	// Build DB config form hook
	dbConfig := &DBConfig{
		Endpoint: hook.Spec.EndPoint,
		Database: d.Database,
		UserName: string(dbSecret.Data["username"]),
		Password: string(dbSecret.Data["password"]),
		Provider: hook.Spec.AppProvider,
	}
	switch d.Operation {
	case StressOperationReplicate:
		err = dbConnect(dbConfig)
		defer dbConfig.db.Close()
		if err != nil {
			return err
		}
		tableReplicateLoop(dbConfig, d.NumLoops, d.TableName)
	}
	return nil
}

func (d *DemoOptions) getHookCR(kubeclient *client.Client, namespace string) (*v1alpha1.AppHook, error) {
	foundHook := &v1alpha1.AppHook{}
	err := kubeclient.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: namespace,
			Name:      d.HookName,
		},
		foundHook)

	if err != nil {
		return nil, err
	}
	if foundHook.Status.Phase == v1alpha1.HookNotReady {
		klog.InfoS("Hook not conected", "hook", d.HookName)
		return nil, nil
	}
	return foundHook, nil
}

func (d *DemoOptions) Run(kubeclient *client.Client) error {
	namespace, err := util.GetOperatorNamespace()
	if err != nil {
		return err
	}

	hook, err := d.getHookCR(kubeclient, namespace)
	if err == nil {
	} else {
		klog.InfoS("Get hook CR failed", "error", err, "hook", d.HookName)
		return err
	}

	err = d.dbStress(kubeclient, hook)
	if err != nil {
		return err
	}
	klog.InfoS("database stress done", "hook", d.HookName)

	return err
}

func main() {
	defer klog.Flush()

	baseName := filepath.Base(os.Args[0])

	c, err := NewCommand(baseName)
	cmd.CheckError(err)
	cmd.CheckError(c.Execute())
}
