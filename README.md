# Endpoint Hitter

## Introduction

Small application that is able to hit in parallel a requested endpoint, having a uuid as a path variable.
The endpoint, the methode type, the number of expected parallel requests, and the authentication credentials can be given as application parameters.

The response status for the uuid and the generated transaction id will be logged in a separate file.
Other application related logs will be sent to stdout.

For example:

We can execute a series of `POST` requests to the endpoint: `https://{env-domain}/__post-publication-combiner/{uuid}`, for the uuids read from `{uuid_file_name}.txt` (`uuid.txt` being the default).

## Installation

Download the source code, dependencies:

```sh
go build .

./endpoint-hitter [--help]
```

## Deployment in k8s as a job

If you want to run it in k8s first you need to build a docker image:

```sh
docker build -t coco/endpoint-hitter:latest .
```

and push it in docker hub:

```sh
docker push coco/endpoint-hitter:latest
```

then make the necessary changes in `./deployment/job.yaml` and deploy the job:

```sh
kubectl apply -f ./deployment/job.yaml
```

when the job is done you can delete it via:

```sh
kubectl delete job endpoint-hitter
```
