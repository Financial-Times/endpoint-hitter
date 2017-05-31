# Endpoint Hitter

## Introduction

Small application that is able to hit in parallel a requested endpoint, having a uuid as a path variable.
The endpoint, the methode type, the number of expected parallel requests, and the authentication credentials can be given as application parameters.

The response status for the uuid and the generated transaction id will be logged in a separate file.
Other application related logs will be sent to stdout.

For example:

We can execute a series of `POST` requests for the endpoint: `https://{env-domain}/__post-publication-combiner/{uuid}`, for the uuids read from `uuids.txt`.
In this case, the response logs will appear in `uuids.log`, with the following format:

```
[34mINFO[0m[0000] Content with uuid: 4bb294f6-2b0f-11e5-1875-703401c316fe for transaction tid_Wz0NP5DaGS_endpoint-hitter ended with status code: 200 
[34mINFO[0m[0000] Content with uuid: 73776692-8bbf-39ab-b2a6-083189b212d8 for transaction tid_O1h15dqVDQ_endpoint-hitter ended with status code: 200 
[34mINFO[0m[0000] Content with uuid: 1637aeb2-4bd8-11e5-b558-8a9722977189 for transaction tid_4PNNLmyWan_endpoint-hitter ended with status code: 200
```

## Installation

Download the source code, dependencies:

        go get -u github.com/Financial-Times/endpoint-hitter
        cd $GOPATH/src/github.com/Financial-Times/endpoint-hitter
        go build .

## Running locally

1. Run the tests and install the binary:

        go install

2. Run the binary (using the `help` flag to see the available optional arguments):

        $GOPATH/bin/endpoint-hitter [--help]
