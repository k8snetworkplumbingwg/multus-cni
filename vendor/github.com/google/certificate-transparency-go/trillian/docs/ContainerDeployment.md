# CT Log Deployment (Containerized)

This document provides links to instructions for deploying a Trillian-based
Certificate Transparency (CT) Log using containers on a cloud platform; this
make the steps and components from the
[Manual Deployment](ManualDeployment.md) document more convenient and
automated, but the same principles apply.

## Core Trillian Services

As a first step, the core
[Trillian cloud deployment](https://github.com/google/trillian/tree/master/examples/deployment/README.md)
document describes how to run a local Docker deployment of the core Trillian
services together with a storage component (MySQL), based on sample
[Docker files](https://github.com/google/trillian/blob/master/examples/deployment/docker)
and a
[Docker Compose manifest](https://github.com/google/trillian/blob/master/examples/deployment/docker-compose.yml).

Further instructions describe how to deploy the core Trillian components on
[Google Cloud Platform (GCP)](https://github.com/google/trillian/blob/master/examples/deployment/kubernetes/README.md),
and on [Amazon Web Services (AWS)](https://github.com/google/trillian/blob/master/examples/deployment/aws/README.md).

## CTFE Personality

The `examples/deployment/` subdirectory includes
[Kubernetes instructions](../examples/deployment/kubernetes/README.md) for
deploying a CTFE personality on top of an existing deployment of the Trillian
core services.
