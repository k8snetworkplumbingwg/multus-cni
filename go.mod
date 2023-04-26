module gopkg.in/k8snetworkplumbingwg/multus-cni.v3

go 1.17

require (
	github.com/containernetworking/cni v0.8.1
	github.com/containernetworking/plugins v0.9.1
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.1.1-0.20210510153419-66a699ae3b05
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.3
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.1 // indirect
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	golang.org/x/net v0.9.0
	google.golang.org/grpc v1.38.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	k8s.io/api v0.22.16
	k8s.io/apimachinery v0.22.16
	k8s.io/client-go v0.22.16
	k8s.io/klog v1.0.0
	k8s.io/kubelet v0.22.16
	k8s.io/kubernetes v1.22.16
)

require (
	github.com/Microsoft/go-winio v0.4.15 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/evanphx/json-patch v4.11.0+incompatible // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-logr/logr v0.4.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.8 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/imdario/mergo v0.3.15 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.26.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	golang.org/x/oauth2 v0.7.0 // indirect
	golang.org/x/sys v0.7.0 // indirect
	golang.org/x/term v0.7.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiserver v0.22.16 // indirect
	k8s.io/component-base v0.22.16 // indirect
	k8s.io/klog/v2 v2.9.0 // indirect
	k8s.io/kube-openapi v0.0.0-20211110012726-3cc51fd1e909 // indirect
	k8s.io/utils v0.0.0-20211116205334-6203023598ed // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.2
	k8s.io/api => k8s.io/api v0.22.16
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.22.16
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.16
	k8s.io/apiserver => k8s.io/apiserver v0.22.16
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.22.16
	k8s.io/client-go => k8s.io/client-go v0.22.16
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.22.16
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.22.16
	k8s.io/code-generator => k8s.io/code-generator v0.22.16
	k8s.io/component-base => k8s.io/component-base v0.22.16
	k8s.io/component-helpers => k8s.io/component-helpers v0.22.16
	k8s.io/controller-manager => k8s.io/controller-manager v0.22.16
	k8s.io/cri-api => k8s.io/cri-api v0.22.16
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.22.16
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.22.16
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.22.16
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.22.16
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.22.16
	k8s.io/kubectl => k8s.io/kubectl v0.22.16
	k8s.io/kubelet => k8s.io/kubelet v0.22.16
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.22.16
	k8s.io/metrics => k8s.io/metrics v0.22.16
	k8s.io/mount-utils => k8s.io/mount-utils v0.22.16
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.22.16
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.22.16
)
