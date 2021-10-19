# app-hook-operator
Kubernetes application backup hook, which can do quiesce and unquiesce of databases during application backup

## Installation
1. Download the yaml file in deploy folder
2. kubectl apply -f deploy

## Supported databases
1. Postgres
2. Mongodb
3. MySql

## Usage
There are examples in config/samples

### appHook CR spec 

| Param | Type | Supported values | Description |
| ----------- | ----------- | ----------- | ----------- |
| AppProvider| string| Postgres / Mongodb / MySql| DB type|
| EndPoint | string | serviceName.namespace |Endpoint to connect the applicatio service|
|Databases | []string | any | database name array|
|OperationType | string | quiesce / unquiesce ||
|TimeoutSeconds | *int32 | >=0 | timeout of operation|
|Secret |corev1.SecretReference | name: xxx, namespace: xxx | Secret to access the database|

### Status

| status | Description |
| ---------------- | --------------------- |
| Created | CR is just created with operationType is empty. This status is short, as manager is connecting and will update the status to ready/not ready|
| Ready | driver manager connected to database successfully. only in Ready status, user can do quiesce operation|
| Not Ready | driver manager failed to connect database. user need to check the spec and fill the correct info|
| Quiesce In Progress | driver is trying to quiesce database|
| Quiesced | databases are successfully quiesced|
| Unquiesce In Progress | driver is trying to unquiesce database|
| Unquiesced | databases are successfully unquiesced|