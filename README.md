# AmberApp
AmberApp is a K8s native framework for application consistency that can work together with Velero and other backup solutions. 

![](https://gitee.com/jibutech/tech-docs/raw/master/images/amberapp-architecture.png)

## Installation
1. Clone the repo

    `git clone git@github.com:jibudata/amberapp.git`

2. Enter the repo and run

    `kubectl apply -f deploy`

## Supported databases
1. PostgreSQL
2. MongoDB
3. MySQL

## Usage
### CLI example
1. Clone repo, do install as above, run `make` to build binaries
2. Create an hook to MySQL database:
```bash
# bin/apphook create -n test -a mysql -e "wordpress-mysql.wordpress" -u root -p passw0rd --databases mysql

# kubectl get apphook -n amberapp-system test-hook
NAME        AGE   CREATED AT             PHASE
test-hook   8s    2021-10-20T12:26:28Z   Ready
```
3. Quiesce DB:
```bash
# bin/apphook quiesce -n test -w

# kubectl get apphook -n amberapp-system test-hook
test-hook   18m   2021-10-20T12:26:28Z   Quiesced
```
4. Unquiesce DB:
```bash
# bin/apphook unquiesce -n test

# kubectl get apphook -n amberapp-system test-hook
test-hook   18m   2021-10-20T12:26:28Z   Unquiesced
```
5. Delete hook:
```bash
# bin/apphook delete -n test
```

### Use CR
Other backup solution can use CR for API level integration with AmberApp, below are CR details.

#### CR spec 

| Param | Type | Supported values | Description |
| ----------- | ----------- | ----------- | ----------- |
| AppProvider| string| Postgres / Mongodb / MySql| DB type|
| EndPoint | string | serviceName.namespace |Endpoint to connect the applicatio service|
|Databases | []string | any | database name array|
|OperationType | string | quiesce / unquiesce ||
|TimeoutSeconds | *int32 | >=0 | timeout of operation|
|Secret |corev1.SecretReference | name: xxx, namespace: xxx | Secret to access the database|

#### Status

| status | Description |
| ---------------- | --------------------- |
| Created | CR is just created with operationType is empty. This status is short, as manager is connecting and will update the status to ready/not ready|
| Ready | driver manager connected to database successfully. only in Ready status, user can do quiesce operation|
| Not Ready | driver manager failed to connect database. user need to check the spec and fill the correct info|
| Quiesce In Progress | driver is trying to quiesce database|
| Quiesced | databases are successfully quiesced|
| Unquiesce In Progress | driver is trying to unquiesce database|
| Unquiesced | databases are successfully unquiesced|

