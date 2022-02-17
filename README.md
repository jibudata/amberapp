# AmberApp

AmberApp is a K8s native framework for application consistency that can work together with Velero and other backup solutions. It will lock databases when backup PVCs.

![overview](https://gitee.com/jibutech/tech-docs/raw/master/images/amberapp-architecture.png)

## Installation

1. Clone the repo

   `git clone git@github.com:jibudata/amberapp.git`

2. Enter the repo and run

   `kubectl apply -f deploy`

## Supported databases

| #   | Type       | Databases required | lock method                 | description                                                                   |
| --- | ---------- | ------------------ | --------------------------- | ----------------------------------------------------------------------------- |
| 1.  | PostgreSQL | y                  | pg_start_backup             | no impact on CRUD                                                             |
| 2.  | MongoDB    | n                  | fsync lock                  | lock all DBs in current user, db modify operatrion will hang until unquiesced |
| 3.  | MySQL      | y                  | FLUSH TABLES WITH READ LOCK | lock all DBs, cannot create new table, insert or modify data until unquiesced |

## Usage

### CLI example

1. Clone repo, do install as above, run `make` to build binaries
2. Deploy an example application: wordpress, refer to <https://github.com/jibutech/docs/tree/main/examples/workload/wordpress>
3. Create an hook to MySQL database. NOTE: use `WATCH_NAMESPACE` to specify the namespace where amberapp operator is installed.

   ```bash
   # export WATCH_NAMESPACE=amberapp-system
   # bin/apphook create -n test -a mysql -e "wordpress-mysql.wordpress" -u root -p passw0rd --databases mysql

   # kubectl get apphooks.ys.jibudata.com -n amberapp-system test-hook
   NAME        AGE   CREATED AT             PHASE
   test-hook   8s    2021-10-20T12:26:28Z   Ready
   ```

4. Quiesce DB:

   ```bash
   # bin/apphook quiesce -n test -w

   # kubectl get apphooks.ys.jibudata.com -n amberapp-system test-hook
   test-hook   18m   2021-10-20T12:26:28Z   Quiesced
   ```

5. Unquiesce DB:

   ```bash
   # bin/apphook unquiesce -n test

   # kubectl get apphooks.ys.jibudata.com -n amberapp-system test-hook
   test-hook   18m   2021-10-20T12:26:28Z   Unquiesced
   ```

6. Delete hook:

   ```bash
   # bin/apphook delete -n test
   ```

### Use CR

Other backup solution can use CR for API level integration with AmberApp, below are CR details.

#### CR spec

| Param          | Type                   | Supported values           | Description                                |
| -------------- | ---------------------- | -------------------------- | ------------------------------------------ |
| AppProvider    | string                 | Postgres / Mongodb / MySql | DB type                                    |
| EndPoint       | string                 | serviceName.namespace      | Endpoint to connect the applicatio service |
| Databases      | []string               | any                        | database name array                        |
| OperationType  | string                 | quiesce / unquiesce        |                                            |
| TimeoutSeconds | \*int32                | >=0                        | timeout of operation                       |
| Secret         | corev1.SecretReference | name: xxx, namespace: xxx  | Secret to access the database              |

#### Status

| status                | Description                                                                                                                                  |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Created               | CR is just created with operationType is empty. This status is short, as manager is connecting and will update the status to ready/not ready |
| Ready                 | driver manager connected to database successfully. only in Ready status, user can do quiesce operation                                       |
| Not Ready             | driver manager failed to connect database. user need to check the spec and fill the correct info                                             |
| Quiesce In Progress   | driver is trying to quiesce database                                                                                                         |
| Quiesced              | databases are successfully quiesced                                                                                                          |
| Unquiesce In Progress | driver is trying to unquiesce database                                                                                                       |
| Unquiesced            | databases are successfully unquiesced                                                                                                        |

## Development

1. generate all resources

   ```bash
   make generate-all -e VERSION=0.0.5
   ```

2. build docker image

   ```bash
   make docker-build -e VERSION=0.0.5
   ```

3. deploy

   ```bash
   make deploy -e VERSION=0.0.5
   ```
