package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

const (
	GroupName = "net.resource.multus-cni.io"
	Version   = "v1alpha1"

	NetConfigKind = "NetConfig"
)

// Decoder implements a decoder for objects in this API group.
var Decoder runtime.Decoder

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetConfig holds the set of parameters for configuring Multus via DRA.
type NetConfig struct {
	metav1.TypeMeta `json:",inline"`

	// Networks is a string matching the format used in the Multus annotation,
	// e.g. "default/macvlan-conf", "macvlan-conf", or JSON array.
	Networks string `json:"networks"`
}

// DefaultNetConfig provides a default NetConfig object.
func DefaultNetConfig() *NetConfig {
	return &NetConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: GroupName + "/" + Version,
			Kind:       NetConfigKind,
		},
		Networks: "",
	}
}

// Normalize updates a NetConfig with default values if needed.
func (c *NetConfig) Normalize() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	// You could normalize single-net formats here if needed.
	return nil
}

// Validate performs basic validation on the NetConfig.
func (c *NetConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if c.Networks == "" {
		return fmt.Errorf("networks must not be empty")
	}
	return nil
}

func init() {
	scheme := runtime.NewScheme()
	schemeGroupVersion := schema.GroupVersion{
		Group:   GroupName,
		Version: Version,
	}
	scheme.AddKnownTypes(schemeGroupVersion,
		&NetConfig{},
	)
	metav1.AddToGroupVersion(scheme, schemeGroupVersion)

	Decoder = json.NewSerializerWithOptions(
		json.DefaultMetaFactory,
		scheme,
		scheme,
		json.SerializerOptions{
			Pretty: true, Strict: true,
		},
	)
}
