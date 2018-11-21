// Copyright (c) 2018 Intel Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/types"

	"github.com/containernetworking/cni/libcni"
	"k8s.io/api/admission/v1beta1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type jsonPatchOperation struct {
	Operation string      `json:"op"`
	Path      string      `json:"path"`
	Value     interface{} `json:"value,omitempty"`
}

const (
	networksAnnotationKey  = "k8s.v1.cni.cncf.io/networks"
	networkResourceNameKey = "k8s.v1.cni.cncf.io/resourceName"
)

var (
	clientset kubernetes.Interface
)

func validateNetworkAttachmentDefinition(netAttachDef types.NetworkAttachmentDefinition) (bool, error) {
	nameRegex := `^[a-z-1-9]([-a-z0-9]*[a-z0-9])?$`
	isNameCorrect, err := regexp.MatchString(nameRegex, netAttachDef.Metadata.Name)
	if !isNameCorrect {
		err := logging.Errorf("invalid name")
		return false, err
	}
	if err != nil {
		err := logging.Errorf("error validating name: %s", err)
		return false, err
	}

	if netAttachDef.Spec.Config == "" {
		err := logging.Errorf("network Config is empty")
		return false, err
	}

	logging.Debugf("validating network config spec: %s", netAttachDef.Spec.Config)

	/* try to unmarshal config into NetworkConfig or NetworkConfigList
	   using actual code from libcni - if succesful, it means that the config
	   will be accepted by CNI itself as well */
	confBytes := []byte(netAttachDef.Spec.Config)
	_, err = libcni.ConfListFromBytes(confBytes)
	if err != nil {
		logging.Debugf("spec is not a valid network config: %s... trying to parse into config list", err)
		_, err = libcni.ConfFromBytes(confBytes)
		if err != nil {
			logging.Debugf("spec is not a valid network config list: %s", confBytes)
			err := logging.Errorf("invalid config: %s", err)
			return false, err
		}
	}

	logging.Debugf("AdmissionReview request allowed: Network Attachment Definition is valid")
	return true, nil
}

func prepareAdmissionReviewResponse(allowed bool, message string, ar *v1beta1.AdmissionReview) error {
	if ar.Request != nil {
		ar.Response = &v1beta1.AdmissionResponse{
			UID:     ar.Request.UID,
			Allowed: allowed,
		}
		if message != "" {
			ar.Response.Result = &metav1.Status{
				Message: message,
			}
		}
		return nil
	}
	return fmt.Errorf("received empty AdmissionReview request")
}

func readAdmissionReview(req *http.Request) (*v1beta1.AdmissionReview, int, error) {
	var body []byte

	if req.Body != nil {
		if data, err := ioutil.ReadAll(req.Body); err == nil {
			body = data
		}
	}

	if len(body) == 0 {
		err := logging.Errorf("Error reading HTTP request: empty body")
		return nil, http.StatusBadRequest, err
	}

	/* validate HTTP request headers */
	contentType := req.Header.Get("Content-Type")
	if contentType != "application/json" {
		err := logging.Errorf("Invalid Content-Type='%s', expected 'application/json'", contentType)
		return nil, http.StatusUnsupportedMediaType, err
	}

	/* read AdmissionReview from the request body */
	ar, err := deserializeAdmissionReview(body)
	if err != nil {
		err := logging.Errorf("error deserializing AdmissionReview: %s", err.Error())
		return nil, http.StatusBadRequest, err
	}

	return ar, http.StatusOK, nil
}

func deserializeAdmissionReview(body []byte) (*v1beta1.AdmissionReview, error) {
	ar := &v1beta1.AdmissionReview{}
	runtimeScheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(runtimeScheme)
	deserializer := codecs.UniversalDeserializer()
	_, _, err := deserializer.Decode(body, nil, ar)

	/* Decode() won't return an error if the data wasn't actual AdmissionReview */
	if err == nil && ar.TypeMeta.Kind != "AdmissionReview" {
		err = fmt.Errorf("received object is not an AdmissionReview")
	}

	return ar, err
}

func deserializeNetworkAttachmentDefinition(ar *v1beta1.AdmissionReview) (types.NetworkAttachmentDefinition, error) {
	/* unmarshal NetworkAttachmentDefinition from AdmissionReview request */
	netAttachDef := types.NetworkAttachmentDefinition{}
	err := json.Unmarshal(ar.Request.Object.Raw, &netAttachDef)
	return netAttachDef, err
}

func deserializePod(ar *v1beta1.AdmissionReview) (v1.Pod, error) {
	/* unmarshal Pod from AdmissionReview request */
	pod := v1.Pod{}
	err := json.Unmarshal(ar.Request.Object.Raw, &pod)
	return pod, err
}

func handleValidationError(w http.ResponseWriter, ar *v1beta1.AdmissionReview, orgErr error) {
	err := prepareAdmissionReviewResponse(false, orgErr.Error(), ar)
	if err != nil {
		err := logging.Errorf("error preparing AdmissionResponse: %s", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeResponse(w, ar)
}

func writeResponse(w http.ResponseWriter, ar *v1beta1.AdmissionReview) {
	logging.Debugf("sending response to the Kubernetes API server")
	resp, _ := json.Marshal(ar)
	w.Write(resp)
}

func validateHandler(w http.ResponseWriter, req *http.Request) {
	/* read AdmissionReview from the HTTP request */
	ar, httpStatus, err := readAdmissionReview(req)
	if err != nil {
		http.Error(w, err.Error(), httpStatus)
		return
	}

	netAttachDef, err := deserializeNetworkAttachmentDefinition(ar)
	if err != nil {
		handleValidationError(w, ar, err)
		return
	}

	/* perform actual object validation */
	allowed, err := validateNetworkAttachmentDefinition(netAttachDef)
	if err != nil {
		handleValidationError(w, ar, err)
		return
	}

	/* perpare response and send it back to the API server */
	err = prepareAdmissionReviewResponse(allowed, "", ar)
	if err != nil {
		logging.Errorf(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeResponse(w, ar)
}

func getNetworkAttachmentDefinition(namespace, name string) (*types.NetworkAttachmentDefinition, error) {
	path := fmt.Sprintf("/apis/k8s.cni.cncf.io/v1/namespaces/%s/network-attachment-definitions/%s", namespace, name)
	rawNetworkAttachmentDefinition, err := clientset.ExtensionsV1beta1().RESTClient().Get().AbsPath(path).DoRaw()
	if err != nil {
		err := logging.Errorf("could not get Network Attachment Definition %s/%s: %s", namespace, name, err)
		return nil, err
	}

	networkAttachmentDefinition := types.NetworkAttachmentDefinition{}
	json.Unmarshal(rawNetworkAttachmentDefinition, &networkAttachmentDefinition)

	return &networkAttachmentDefinition, nil
}

func parsePodNetworkSelectionElement(selection, defaultNamespace string) (*types.NetworkSelectionElement, error) {
	var namespace, name, netInterface string
	var networkSelectionElement *types.NetworkSelectionElement

	units := strings.Split(selection, "/")
	switch len(units) {
	case 1:
		namespace = defaultNamespace
		name = units[0]
	case 2:
		namespace = units[0]
		name = units[1]
	default:
		return networkSelectionElement, logging.Errorf("invalid network selection element - more than one '/' rune in: %s", selection)
	}

	units = strings.Split(name, "@")
	switch len(units) {
	case 1:
		name = units[0]
		netInterface = ""
	case 2:
		name = units[0]
		netInterface = units[1]
	default:
		return networkSelectionElement, logging.Errorf("invalid network selection element - more than one '@' rune in: %s", selection)
	}

	validNameRegex, _ := regexp.Compile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	for _, unit := range []string{namespace, name, netInterface} {
		ok := validNameRegex.MatchString(unit)
		if !ok && len(unit) > 0 {
			return networkSelectionElement, logging.Errorf("at least one of the network selection units is invalid: error found at '%s'", unit)
		}
	}

	networkSelectionElement = &types.NetworkSelectionElement{
		Namespace:        namespace,
		Name:             name,
		InterfaceRequest: netInterface,
	}

	return networkSelectionElement, nil
}

func parsePodNetworkSelections(podNetworks, defaultNamespace string) ([]*types.NetworkSelectionElement, error) {
	var networkSelections []*types.NetworkSelectionElement

	if len(podNetworks) == 0 {
		return nil, logging.Errorf("empty string passed as network selection elements list")
	}

	/* try to parse as JSON array */
	err := json.Unmarshal([]byte(podNetworks), &networkSelections)

	/* if failed, try to parse as comma separated */
	if err != nil {
		logging.Debugf("%s is not in JSON format: %s. Will try to parse as comma separated network selections list", podNetworks, err.Error())
		for _, networkSelection := range strings.Split(podNetworks, ",") {
			networkSelection = strings.TrimSpace(networkSelection)
			networkSelectionElement, err := parsePodNetworkSelectionElement(networkSelection, defaultNamespace)
			if err != nil {
				return nil, logging.Errorf("error parsing network selection element: %v", err)
			}
			networkSelections = append(networkSelections, networkSelectionElement)
		}
	}

	/* fill missing namespaces with default value */
	for _, networkSelection := range networkSelections {
		if networkSelection.Namespace == "" {
			networkSelection.Namespace = defaultNamespace
		}
	}

	return networkSelections, nil
}

func mutateHandler(w http.ResponseWriter, req *http.Request) {
	logging.Debugf("Received mutation request")

	/* read AdmissionReview from the HTTP request */
	ar, httpStatus, err := readAdmissionReview(req)
	if err != nil {
		http.Error(w, err.Error(), httpStatus)
		return
	}

	/* read pod annotations */
	/* if networks missing skip everything */
	pod, err := deserializePod(ar)
	if err != nil {
		handleValidationError(w, ar, err)
		return
	}
	if netSelections, exists := pod.ObjectMeta.Annotations[networksAnnotationKey]; exists && netSelections != "" {
		/* map of resources request needed by a pod and a number of them */
		resourceRequests := make(map[string]int64)

		/* unmarshal list of network selection objects */
		networks, _ := parsePodNetworkSelections(netSelections, pod.ObjectMeta.Namespace)

		for _, n := range networks {
			/* for each network in annotation ask API server for network-attachment-definition */
			networkAttachmentDefinition, err := getNetworkAttachmentDefinition(n.Namespace, n.Name)
			if err != nil {
				/* if doesn't exist: deny pod */
				explanation := logging.Errorf("could not find network attachment definition '%s/%s': %s", n.Namespace, n.Name, err)
				err = prepareAdmissionReviewResponse(false, explanation.Error(), ar)
				if err != nil {
					logging.Errorf("error preparing AdmissionReview response: %s", err)
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeResponse(w, ar)
				return
			}
			logging.Debugf("network attachment definition '%s/%s' found", n.Namespace, n.Name)

			/* network object exists, so check if it contains resourceName annotation */
			if resourceName, exists := networkAttachmentDefinition.Metadata.Annotations[networkResourceNameKey]; exists {
				/* add resource to map/increment if it was already there */
				resourceRequests[resourceName]++
				logging.Debugf("resource '%s' needs to be requested for network '%s/%s'", resourceName, n.Name, n.Namespace)
			} else {
				logging.Debugf("network '%s/%s' doesn't use custom resources, skipping...", n.Namespace, n.Name)
			}
		}

		/* patch with custom resources requests and limits */
		err = prepareAdmissionReviewResponse(true, "allowed", ar)
		if err != nil {
			logging.Errorf("error preparing AdmissionReview response: %s", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(resourceRequests) == 0 {
			logging.Debugf("pod doesn't need any custom network resources")
		} else {
			var patch []jsonPatchOperation

			resourceList := v1.ResourceList{}
			for name, number := range resourceRequests {
				resourceList[v1.ResourceName(name)] = *resource.NewQuantity(number, resource.DecimalSI)
			}

			patch = append(patch, jsonPatchOperation{
				Operation: "add",
				Path:      "/spec/containers/0/resources/requests", // NOTE: in future we may want to patch specific container (not always the first one)
				Value:     resourceList,
			})
			patch = append(patch, jsonPatchOperation{
				Operation: "add",
				Path:      "/spec/containers/0/resources/limits",
				Value:     resourceList,
			})
			patchBytes, _ := json.Marshal(patch)
			ar.Response.Patch = patchBytes
		}
	} else {
		/* network annoation not provided or empty */
		logging.Debugf("pod spec doesn't have network annotations. Skipping...")
		err = prepareAdmissionReviewResponse(true, "Pod spec doesn't have network annotations. Skipping...", ar)
		if err != nil {
			logging.Errorf("error preparing AdmissionReview response: %s", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	writeResponse(w, ar)
	return

}

func main() {
	/* load configuration */
	port := flag.Int("port", 443, "The port on which to serve.")
	address := flag.String("bind-address", "0.0.0.0", "The IP address on which to listen for the --port port.")
	cert := flag.String("tls-cert-file", "cert.pem", "File containing the default x509 Certificate for HTTPS.")
	key := flag.String("tls-private-key-file", "key.pem", "File containing the default x509 private key matching --tls-cert-file.")
	flag.Parse()

	/* enable logging */
	logging.SetLogLevel("debug")
	logging.Debugf("starting Multus webhook server")

	/* register handlers */
	http.HandleFunc("/validate", validateHandler)
	http.HandleFunc("/mutate", mutateHandler)

	/* setup Kubernetes API client */
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	/* start serving */
	err = http.ListenAndServeTLS(fmt.Sprintf("%s:%d", *address, *port), *cert, *key, nil)
	if err != nil {
		logging.Errorf("error starting web server: %s", err.Error())
		return
	}
}
