module gopkg.in/k8snetworkplumbingwg/multus-cni.v3

go 1.16

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/containernetworking/cni v1.0.1
	github.com/containernetworking/plugins v1.1.0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/go-logr/logr v1.2.2 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.2.0
	github.com/kr/text v0.2.0 // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/pkg/errors v0.9.1
	github.com/vishvananda/netlink v1.1.1-0.20210330154013-f5de75959ad5
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f
	golang.org/x/sys v0.0.0-20220227234510-4e6760a101f9 // indirect
	google.golang.org/grpc v1.33.2
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	k8s.io/api v0.20.10
	k8s.io/apimachinery v0.20.10
	k8s.io/client-go v0.20.10
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.40.1 // indirect
	k8s.io/kube-openapi v0.0.0-20220124234850-424119656bbf // indirect
	k8s.io/kubelet v0.0.0
	k8s.io/kubernetes v1.20.10
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.2
	k8s.io/api => k8s.io/api v0.20.10
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.20.10
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.10
	k8s.io/apiserver => k8s.io/apiserver v0.20.10
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.20.10
	k8s.io/client-go => k8s.io/client-go v0.20.10
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.20.10
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.20.10
	k8s.io/code-generator => k8s.io/code-generator v0.20.10
	k8s.io/component-base => k8s.io/component-base v0.20.10
	k8s.io/component-helpers => k8s.io/component-helpers v0.20.10
	k8s.io/controller-manager => k8s.io/controller-manager v0.20.10
	k8s.io/cri-api => k8s.io/cri-api v0.20.10
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.20.10
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.20.10
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.20.10
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.20.10
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.20.10
	k8s.io/kubectl => k8s.io/kubectl v0.20.10
	k8s.io/kubelet => k8s.io/kubelet v0.20.10
	k8s.io/kubernetes => k8s.io/kubernetes v1.20.10
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.20.10
	k8s.io/metrics => k8s.io/metrics v0.20.10
	k8s.io/mount-utils => k8s.io/mount-utils v0.20.10
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.20.10
)
