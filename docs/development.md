## Development/Support Information

## Which Kubernetes version is supported in multus?

Currently multus team supports Kubernetes that Kubernetes community maintains.
See [Version Skew Policy](https://kubernetes.io/releases/version-skew-policy/) for the details.

## How to debug multus-cni thin image?

Latest multus uses [distroless](https://github.com/GoogleContainerTools/distroless) container image for its base,
hence there is no shell command. If you want to execute shell in multus pod, please use `-debug` image (e.g. ghcr.io/k8snetworkplumbingwg/multus-cni:snapshot-debug), which has shell.

## How to utilize multus-cni code as library?

Multus now uses [gopkg.in](http://gopkg.in/) to expose its code as library.
You can use following command to import our code into your go code.

```
go get gopkg.in/k8snetworkplumbingwg/multus-cni.v4
```

## How do I submit an issue?

Use GitHub as normally, you'll be presented with an option to submit a issue or enhancement request.

Issues are considered stale after 90 days. After which, the maintainers reserve the right to close an issue.

Typically, we'll tag the submitter and ask for more information if necessary before closing.

If an issue is closed that you don't feel is sufficiently resolved, please feel free to re-open the issue and provide any necessary information.

## How do I build multus-cni?

You can use the built in `./hack/build-go.sh` script!

```
git clone https://github.com/k8snetworkplumbingwg/multus-cni.git
cd multus-cni
./hack/build-go.sh
```

## How do I run CI tests?

Multus has go unit tests (based on ginkgo framework).The following commands drive CI tests manually in your environment:

```
sudo ./hack/test-go.sh
```

## What are the best practices for logging?

The following are the best practices for multus logging:

* Add `logging.Debugf()` at the beginning of functions
* In case of error handling, use `logging.Errorf()` with given error info
* `logging.Panicf()` only be used for critical errors (it should NOT normally be used)


## Multus release schedule

On the first maintainer's meeting, twice yearly, after January 1st and July 1st, if a new version has not been tagged, a new version will tagged.

## Multi-arch builds

Multus is currently built for a number of architectures, however, our testing and validation is only performed against x86 architectures. Our x86 architecture has end to end testing, however, for other architectures, only supported via best effort community contributions.
