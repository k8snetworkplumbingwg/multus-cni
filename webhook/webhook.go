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

	"github.com/intel/multus-cni/logging"
	"github.com/intel/multus-cni/types"

	"github.com/containernetworking/cni/libcni"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

func validateNetworkAttachmentDefinition(netAttachDef types.NetworkAttachmentDefinition) (bool, error) {
	nameRegex := `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	isNameCorrect, err := regexp.MatchString(nameRegex, netAttachDef.Metadata.Name)
	if !isNameCorrect {
		logging.Errorf("Invalid name.")
		return false, fmt.Errorf("Invalid name")
	}
	if err != nil {
		logging.Errorf("Error validating name: %s.", err)
		return false, err
	}

	if netAttachDef.Spec.Config == "" {
		logging.Errorf("Network Config is empty.")
		return false, fmt.Errorf("Network Config is empty")
	}

	logging.Printf(logging.DebugLevel, "Validating network config spec: %s", netAttachDef.Spec.Config)

	/* try to unmarshal config into NetworkConfig or NetworkConfigList
	   using actual code from libcni - if succesful, it means that the config
		 will be accepted by CNI itseld as well */
	confBytes := []byte(netAttachDef.Spec.Config)
	_, err = libcni.ConfListFromBytes(confBytes)
	if err != nil {
		logging.Printf(logging.DebugLevel, "Spec is not a valid network config: %s. Trying to parse into config list", err)
		_, err = libcni.ConfFromBytes(confBytes)
		if err != nil {
			logging.Printf(logging.DebugLevel, "Spec is not a valid network config list: %s", err)
			logging.Errorf("Invalid config: %s", err)
			return false, fmt.Errorf("Invalid network config spec")
		}
	}

	logging.Printf(logging.DebugLevel, "Network Attachment Defintion is valid. Admission Review request allowed")
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
	} else {
		return fmt.Errorf("AdmissionReview request empty")
	}
}

func deserializeAdmissionReview(body []byte) (v1beta1.AdmissionReview, error) {
	ar := v1beta1.AdmissionReview{}
	runtimeScheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(runtimeScheme)
	deserializer := codecs.UniversalDeserializer()
	_, _, err := deserializer.Decode(body, nil, &ar)

	/* Decode() won't return an error if the data wasn't actual AdmissionReview */
	if err == nil && ar.TypeMeta.Kind != "AdmissionReview" {
		err = fmt.Errorf("Object is not an AdmissionReview")
	}

	return ar, err
}

func deserializeNetworkAttachmentDefinition(ar v1beta1.AdmissionReview) (types.NetworkAttachmentDefinition, error) {
	/* unmarshal NetworkAttachmentDefinition from AdmissionReview request */
	netAttachDef := types.NetworkAttachmentDefinition{}
	err := json.Unmarshal(ar.Request.Object.Raw, &netAttachDef)
	return netAttachDef, err
}

func handleValidationError(w http.ResponseWriter, ar v1beta1.AdmissionReview, orgErr error) {
	err := prepareAdmissionReviewResponse(false, orgErr.Error(), &ar)
	if err != nil {
		logging.Errorf("Error preparing AdmissionResponse: %s", err.Error())
		http.Error(w, fmt.Sprintf("Error preparing AdmissionResponse: %s", err.Error()), http.StatusBadRequest)
		return
	}
	writeResponse(w, ar)
}

func writeResponse(w http.ResponseWriter, ar v1beta1.AdmissionReview) {
	logging.Printf(logging.DebugLevel, "Sending response to the API server")
	resp, _ := json.Marshal(ar)
	w.Write(resp)
}

func validateHandler(w http.ResponseWriter, req *http.Request) {
	var body []byte

	if req.Body != nil {
		if data, err := ioutil.ReadAll(req.Body); err == nil {
			body = data
		}
	}

	if len(body) == 0 {
		logging.Errorf("Error reading HTTP request: empty body")
		http.Error(w, "Error reading HTTP request: empty body", http.StatusBadRequest)
		return
	}

	/* validate HTTP request headers */
	contentType := req.Header.Get("Content-Type")
	if contentType != "application/json" {
		logging.Errorf("Invalid Content-Type='%s', expected 'application/json'", contentType)
		http.Error(w, "Invalid Content-Type='%s', expected 'application/json'", http.StatusUnsupportedMediaType)
		return
	}

	/* read AdmissionReview from the request body */
	ar, err := deserializeAdmissionReview(body)
	if err != nil {
		logging.Errorf("Error deserializing AdmissionReview: %s", err.Error())
		http.Error(w, fmt.Sprintf("Error deserializing AdmissionReview: %s", err.Error()), http.StatusBadRequest)
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

	err = prepareAdmissionReviewResponse(allowed, "", &ar)
	if err != nil {
		logging.Errorf(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeResponse(w, ar)
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
	logging.Printf(logging.DebugLevel, "Starting Multus webhook server")

	/* register handlers */
	http.HandleFunc("/validate", validateHandler)

	/* start serving */
	err := http.ListenAndServeTLS(fmt.Sprintf("%s:%d", *address, *port), *cert, *key, nil)
	if err != nil {
		logging.Errorf("Error starting web server: %s", err.Error())
		return
	}
}
