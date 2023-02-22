module gopkg.in/k8snetworkplumbingwg/multus-cni.v3

go 1.17

require (
	github.com/containernetworking/cni v0.8.1
	github.com/containernetworking/plugins v0.9.1
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.1.1-0.20210510153419-66a699ae3b05
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.3
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.1 // indirect
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	golang.org/x/net v0.1.1-0.20221104162952-702349b0e862
	google.golang.org/grpc v1.27.1
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
	k8s.io/klog v1.0.0
	k8s.io/kubelet v0.0.0
	k8s.io/kubernetes v1.21.5
)

require (
	github.com/Microsoft/go-winio v0.4.15 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/evanphx/json-patch v4.9.0+incompatible // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/go-logr/logr v0.4.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.5.0 // indirect
	github.com/google/go-cmp v0.5.5 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/googleapis/gnostic v0.4.1 // indirect
	github.com/imdario/mergo v0.3.5 // indirect
	github.com/json-iterator/go v1.1.11 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/nxadm/tail v1.4.4 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.26.0 // indirect
	github.com/prometheus/procfs v0.6.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/vishvananda/netns v0.0.0-20200728191858-db3c7e526aae // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/sys v0.1.0 // indirect
	golang.org/x/term v0.1.0 // indirect
	golang.org/x/text v0.4.0 // indirect
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20201110150050-8816d57aaa9a // indirect
	google.golang.org/protobuf v1.26.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/apiserver v0.21.1 // indirect
	k8s.io/component-base v0.21.1 // indirect
	k8s.io/klog/v2 v2.8.0 // indirect
	k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7 // indirect
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.1.2 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace (
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.2
	k8s.io/api => k8s.io/api v0.21.1
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.21.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.21.1
	k8s.io/apiserver => k8s.io/apiserver v0.21.1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.21.1
	k8s.io/client-go => k8s.io/client-go v0.21.1
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.21.1
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.21.1
	k8s.io/code-generator => k8s.io/code-generator v0.21.1
	k8s.io/component-base => k8s.io/component-base v0.21.1
	k8s.io/component-helpers => k8s.io/component-helpers v0.21.1
	k8s.io/controller-manager => k8s.io/controller-manager v0.21.1
	k8s.io/cri-api => k8s.io/cri-api v0.21.1
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.21.1
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.21.1
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.21.1
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.21.1
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.21.1
	k8s.io/kubectl => k8s.io/kubectl v0.21.1
	k8s.io/kubelet => k8s.io/kubelet v0.21.1
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.21.1
	k8s.io/metrics => k8s.io/metrics v0.21.1
	k8s.io/mount-utils => k8s.io/mount-utils v0.21.1
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.21.1
)
