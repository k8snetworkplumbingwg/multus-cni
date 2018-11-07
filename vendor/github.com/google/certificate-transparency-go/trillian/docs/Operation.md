# Operating a CT Log

Once a CT log is deployed it needs to be kept operational, particularly if it
is expected to be included in Chrome's
[list of trusted logs](http://www.certificate-transparency.org/known-logs).

Be **warned**: running a CT log is more difficult than running a normal
database-backed web site, because of the security properties required from a Log
&ndash; running a public Log involves a commitment to reliably store all (valid)
uploaded certificates and include them in the tree within a specified period.

This means that failures that would be recoverable for a normal website &ndash;
losing tiny amounts of logged data, accidentally re-using keys &ndash; will
result in the [failure](https://tools.ietf.org/html/rfc6962#section-7.3) of a CT
Log.

 - [Key Management](#key-management)
 - [Temporal Sharding](#temporal-sharding)
 - [Alerting](#alerting)
 - [Load Testing](#load-testing)
 - [Backups](#backups)
 - [Troubleshooting](#troubleshooting)
 - [Browser Submission](#browser-submission)


## Key Management

A CT Log is a cryptographic entity that signs data using a
[private key](https://tools.ietf.org/html/rfc6962#section-2.1.4).  This key is
needed by all of the distributed CTFE instances, but also needs to be kept
secure.  In particular:

 - The CT Log key must not be re-used for distinct Logs.
 - The CT Log key should not be re-used for HTTPS/TLS termination.

The corresponding public key is needed in order to register as a Log that is
[trusted by browsers](#browser-submission).


## Temporal Sharding

To prevent unbounded growth of Log instances, it is recommended that a new
production Log is set up to be *temporally sharded*: a collection of separate
Log instances (each with its own private key) that each accept certificates
with a `NotAfter` date in a particular date range (usually a calendar year).

The [multi-tenant nature](#ManualDeployment.md#tree-provisioning) of
Trillian-based Logs makes this straightforward to deploy; each shard just needs
to set the [`not_after_start`, `not_after_limit`) range in the
[CTFE configuration files](#ManualDeployment.md#ctfe-configuration).


## Alerting

The deployment documents include discussion of
[monitoring mechanisms](/ManualDeployment.md#monitoring); for reliable
operation, this monitoring should be connected to an alerting system that gives
enough time for operations staff to respond to problems.

This alerting should cover normal operational metrics, such as:
 - Rates of errored requests, categorized according to:
    - read and write paths
    - client-side (4xx) errors and server-side (5xx) errors.
 - Latency distribution of requests.
 - Task health, CPU and memory usage.

However, the alerting should also cover criteria that are specific to running a
CT Log.  In particular, the Log issues signed promises to incorporate
submissions within a fixed time window (the maximum merge delay, or MMD), and
this incorporation relies on a single point of failure (the
[signer](ManualDeployment.md#primary-signer-election)).  As such, there are
some CT-specific metrics that can also be alerted on:

 - The age of the most recent Merkle treee head.
 - The size of the current backlog of unmerged submissions.
 - Per-log instance counts of primary signer instances (which is normally 1,
   can transiently be 0, but should never be > 1).


## Load Testing

The modern Web PKI operates at a much larger scale than it did just a couple of
years ago, and this increase in scale is only likely to accelerate (e.g. with a
shift towards shorter certificate expiration times).

This means that a live production Log needs to be able to cope with large
volumes of submissions, resulting in a tree size of hundreds of millions of
certificates (or more!).

To confirm that this scale is indeed supported, it's a good idea to run load
tests on a Log deployment before launch.  This is typically done in a parallel
test environment that is as close to the live environment as possible (being
careful not to [re-use test keys](#key-management)).

This repository includes a couple of tools to help with this testing.  Firstly,
the
[`preloader` tool](https://github.com/google/certificate-transparency-go/blob/master/preload/preloader)
allows the contents of a source log to be copied into a destination log.  This
tool has command-line options to control its parallelism, but is fundamentally
a single-process executable.

The other load-testing tool is the
[`ct_hammer`](https://github.com/google/certificate-transparency-go/blob/master/trillian/integration/ct_hammer),
which tests all of the
[RFC 6962 entrypoints](https://tools.ietf.org/html/rfc6962#section-4) with both
valid and invalid inputs.

 - For write-path testing, `ct_hammer` relies on the Log under test being
   configured to accept a test root certificate, so that synthetic test
   certificates can be submitted.
 - For convenience, `ct_hammer` accepts the same format of configuration file
   that is used to configure the CTFE.  (However, be careful not to distribute
   a CTFE configuration file that includes non-test
   [private keys](#key-management).)
 - The `--rate_limit` option controls the overall rate limit for the tool.
 - Multiple instances of `ct_hammer` can be run in parallel to allow load
   testing to be scaled up arbitrarily.

These testing tools can also be used to confirm that the Log continues to
operate normally while various maintenance activities &ndash; software
rollouts, machine turndowns, configuration updates, etc. &ndash; are in
progress.


## Backups

For most production systems with persistent data, regular backups are
recommended.  However, the cryptographic nature of a CT Log means that backups
of its data induce a dangerous temptation.

The temptation is this: if you have a backup, at some point you will feel the
urge to perform a **restore** from backup.  If any data has been accepted for
inclusion since that backup (and a signed promise-to-include issued), then
restoring the backup is effectively forking the underlying Merkle tree.  This
breaks the tree's append-only property &ndash; resulting in log
disqualification.


## Troubleshooting

All of the Trillian and CTFE binaries use the
[glog](https://github.com/golang/glog) library for logging, so additional
diagnostic information can be obtained by modifying the glog options, for
example, by enabling `--logtostderr -v 1`.

Other useful glog options for debugging specific problems are:

 - `--vmodule`: increase the logging level selectively in particular
   code files.
 - `--log_backtrace_at`: emit a full stack trace at particular logging
   statements.

Also, the underlying storage system can be queried independently, using the
relevant vendor tool:

 - For MySQL, the command line client can be used, in combination with the
   Trillian
   [database schema](https://github.com/google/trillian/blob/master/storage/mysql/storage.sql).
 - For Cloud Spanner, the
   [console](https://cloud.google.com/spanner/docs/quickstart-console#run_a_query)
   can be used, in combination with the
   [database schema](https://github.com/google/trillian/blob/master/storage/cloudspanner/spanner.sdl).

Obviously, this should be done with **extreme** care for a live database!


## Browser Submission

Various browser vendors now require Web PKI certificates to be logged in some
number of accepted CT logs
(e.g. [Chrome](https://github.com/chromium/ct-policy/blob/master/log_policy.md)
[Apple](https://support.apple.com/en-gb/HT205280)).

Each vendor has its own criteria for admission to the set of accepted Logs,
which is beyond the scope of this document.  However, the set of information
that is likely to be needed for browser acceptance includes:

 - The URL for the Log.
 - The public key for the Log.
 - The maximum merge delay (MMD) that the Log has committed to.
 - Any [temporal shard](#temporal-sharding) ranges.
 - The set of accepted root certificates.
 - The values of any rate limits on external traffic.
