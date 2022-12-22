module gopkg.in/k8snetworkplumbingwg/multus-cni.v3

go 1.18

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/containernetworking/cni v1.1.2
	github.com/containernetworking/plugins v1.1.0
	github.com/fsnotify/fsnotify v1.6.0
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.4.0
	github.com/onsi/ginkgo/v2 v2.5.1
	github.com/onsi/gomega v1.24.0
	github.com/pkg/errors v0.9.1
	github.com/vishvananda/netlink v1.1.1-0.20210330154013-f5de75959ad5
	golang.org/x/net v0.1.0
	golang.org/x/sys v0.2.0
	google.golang.org/grpc v1.40.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	k8s.io/api v0.22.8
	k8s.io/apimachinery v0.22.8
	k8s.io/client-go v0.22.8
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.60.1 // indirect
	k8s.io/kube-openapi v0.0.0-20220413171646-5e7f5fdc6da6 // indirect
	k8s.io/kubelet v0.22.8
	sigs.k8s.io/yaml v1.3.0 // indirect
)

require github.com/prometheus/client_golang v1.12.2

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/googleapis/gnostic v0.5.5 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/vishvananda/netns v0.0.0-20210104183010-2eb08e3e575f // indirect
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f // indirect
	golang.org/x/term v0.1.0 // indirect
	golang.org/x/text v0.4.0 // indirect
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220107163113-42d7afdf6368 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/utils v0.0.0-20211116205334-6203023598ed // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.1 // indirect
)

replace (
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.2
	k8s.io/api => k8s.io/api v0.22.8
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.22.8
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.8
	k8s.io/apiserver => k8s.io/apiserver v0.22.8
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.22.8
	k8s.io/client-go => k8s.io/client-go v0.22.8
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.22.8
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.22.8
	k8s.io/code-generator => k8s.io/code-generator v0.22.8
	k8s.io/component-base => k8s.io/component-base v0.22.8
	k8s.io/component-helpers => k8s.io/component-helpers v0.22.8
	k8s.io/controller-manager => k8s.io/controller-manager v0.22.8
	k8s.io/cri-api => k8s.io/cri-api v0.22.8
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.22.8
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.22.8
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.22.8
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20211109043538-20434351676c
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.22.8
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.22.8
	k8s.io/kubectl => k8s.io/kubectl v0.22.8
	k8s.io/kubelet => k8s.io/kubelet v0.22.8
	k8s.io/kubernetes => k8s.io/kubernetes v1.22.8
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.22.8
	k8s.io/metrics => k8s.io/metrics v0.22.8
	k8s.io/mount-utils => k8s.io/mount-utils v0.22.8
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.22.8
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.22.8
)
