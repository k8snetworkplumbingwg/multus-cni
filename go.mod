module gopkg.in/k8snetworkplumbingwg/multus-cni.v4

go 1.20

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/containernetworking/cni v1.1.2
	github.com/containernetworking/plugins v1.1.0
	github.com/fsnotify/fsnotify v1.6.0
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.4.0
	github.com/onsi/ginkgo/v2 v2.9.1
	github.com/onsi/gomega v1.27.4
	github.com/pkg/errors v0.9.1 // indirect
	github.com/vishvananda/netlink v1.1.1-0.20210330154013-f5de75959ad5
	golang.org/x/net v0.8.0
	golang.org/x/sys v0.6.0
	google.golang.org/grpc v1.53.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	k8s.io/api v0.27.5
	k8s.io/apimachinery v0.27.5
	k8s.io/client-go v1.5.2
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.90.1
	k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f // indirect
	k8s.io/kubelet v0.27.5
	sigs.k8s.io/yaml v1.3.0 // indirect
)

require (
	github.com/prometheus/client_golang v1.14.0
	github.com/spf13/pflag v1.0.5
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.1 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/pprof v0.0.0-20210720184732-4bb14d4b1be1 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/vishvananda/netns v0.0.0-20210104183010-2eb08e3e575f // indirect
	golang.org/x/oauth2 v0.4.0 // indirect
	golang.org/x/term v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/time v0.0.0-20220210224613-90d013bbcef8 // indirect
	golang.org/x/tools v0.7.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/utils v0.0.0-20230209194617-a36077c30491 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace (
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.2
	k8s.io/api => k8s.io/api v0.27.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.27.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.27.5
	k8s.io/apiserver => k8s.io/apiserver v0.27.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.27.5
	k8s.io/client-go => k8s.io/client-go v0.27.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.27.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.27.5
	k8s.io/code-generator => k8s.io/code-generator v0.27.5
	k8s.io/component-base => k8s.io/component-base v0.27.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.27.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.27.5
	k8s.io/cri-api => k8s.io/cri-api v0.27.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.27.5
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.27.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.27.5
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20230501164219-8b0f38b5fd1f
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.27.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.27.5
	k8s.io/kubectl => k8s.io/kubectl v0.27.5
	k8s.io/kubelet => k8s.io/kubelet v0.27.5
	k8s.io/kubernetes => k8s.io/kubernetes v1.22.8
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.27.5
	k8s.io/metrics => k8s.io/metrics v0.27.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.27.5
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.27.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.27.5
)
