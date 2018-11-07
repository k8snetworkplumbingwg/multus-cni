# Trillian CT Personality

This directory holds code and scripts for running a Certificate Transparency
(CT) Log based on the [Trillian](https://github.com/google/trillian) general
transparency Log.

 - [Codebase Structure](#codebase-structure)
 - [Deployment](#deployment)
 - [Operation](#operation)


## Codebase Structure

The main code for the CT personality is held in `trillian/ctfe`; this code
responds to HTTP requests on the
[CT API paths](https://tools.ietf.org/html/rfc6962#section-4) and translates
them to the equivalent gRPC API requests to the Trillian Log.

This obviously relies on the gRPC API definitions at
`github.com/google/trillian`; the code also uses common libraries from the
Trillian project for various things including:
 - exposing monitoring and statistics via an `interface` and corresponding
   Prometheus implementation (`github.com/google/trillian/monitoring/...`)
 - dealing with cryptographic keys (`github.com/google/trillian/crypto/...`).

The `trillian/integration/` directory holds scripts and tests for running the whole
system locally.  In particular:
 - `trillian/integration/ct_integration_test.sh` brings up local processes
   running a Trillian Log server, signer and a CT personality, and exercises the
   complete set of RFC 6962 API entrypoints.
 - `trillian/integration/ct_hammer_test.sh` brings up a complete system and runs
   a continuous randomized test of the CT entrypoints.

These scripts require a local database instance to be configured as described
in the [Trillian instructions](https://github.com/google/trillian#mysql-setup).


## Deployment

Deploying a Trillian-based CT Log involves more than just the code contained
in this directory.

The [Manual Deployment document](docs/ManualDeployment.md) describes the
components and process involved in manually setting up a CT Log instance on
individual machines.

The [Containerized Deployment document](docs/ContainerDeployment.md) describes
the sample container scripts which make CT Log deployment easier and more
automatic.  However, if you're planning to operate a trusted CT Log (rather than
simply experimenting/playing with the code) then you should expect to understand all
of the information in the manual version &ndash; even if you use the
containerized variant for deployment convenience.


## Operation

Once all of the components for a Trillian-based CT Log have been deployed,
log operators need to monitor and maintain the Log. The
[Operation document](docs/Operation.md) describes key considerations and gotchas
for this ongoing process.
