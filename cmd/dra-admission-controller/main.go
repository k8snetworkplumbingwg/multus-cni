/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	admissionv1 "k8s.io/api/admission/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/flags"
	configapi "sigs.k8s.io/dra-example-driver/api/example.com/resource/gpu/v1alpha1"
)

const (
	DriverName = "multus-dra.k8s.cni.cncf.io"
)

var (
	resourceClaimResource = metav1.GroupVersionResource{
		Group:    resourceapi.SchemeGroupVersion.Group,
		Version:  resourceapi.SchemeGroupVersion.Version,
		Resource: "resourceclaims",
	}
	resourceClaimTemplateResource = metav1.GroupVersionResource{
		Group:    resourceapi.SchemeGroupVersion.Group,
		Version:  resourceapi.SchemeGroupVersion.Version,
		Resource: "resourceclaimtemplates",
	}
)

type Flags struct {
	loggingConfig *flags.LoggingConfig

	certFile string
	keyFile  string
	port     int
}

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

func init() {
	utilruntime.Must(admissionv1.AddToScheme(scheme))
}

func main() {
	if err := newApp().Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newApp() *cli.App {
	flags := &Flags{
		loggingConfig: flags.NewLoggingConfig(),
	}
	cliFlags := []cli.Flag{
		&cli.StringFlag{
			Name:        "tls-cert-file",
			Usage:       "File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated after server cert).",
			Destination: &flags.certFile,
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "tls-private-key-file",
			Usage:       "File containing the default x509 private key matching --tls-cert-file.",
			Destination: &flags.keyFile,
			Required:    true,
		},
		&cli.IntFlag{
			Name:        "port",
			Usage:       "Secure port that the webhook listens on",
			Value:       443,
			Destination: &flags.port,
		},
	}
	cliFlags = append(cliFlags, flags.loggingConfig.Flags()...)

	app := &cli.App{
		Name:            "multus-dra-webhook",
		Usage:           "multus-dra-webhook implements a validating admission webhook complementing a DRA driver plugin.",
		ArgsUsage:       " ",
		HideHelpCommand: true,
		Flags:           cliFlags,
		Before: func(c *cli.Context) error {
			if c.Args().Len() > 0 {
				return fmt.Errorf("arguments not supported: %v", c.Args().Slice())
			}
			return flags.loggingConfig.Apply()
		},
		Action: func(c *cli.Context) error {
			server := &http.Server{
				Handler: newMux(),
				Addr:    fmt.Sprintf(":%d", flags.port),
			}
			klog.Info("starting webhook server on", server.Addr)
			return server.ListenAndServeTLS(flags.certFile, flags.keyFile)
		},
	}

	return app
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/validate-resource-claim-parameters", serveResourceClaim)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) {
		_, err := w.Write([]byte("ok"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	return mux
}

func serveResourceClaim(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitResourceClaimParameters)
}

// serve handles the http portion of a request prior to handing to an admit
// function.
func serve(w http.ResponseWriter, r *http.Request, admit func(admissionv1.AdmissionReview) *admissionv1.AdmissionResponse) {
	var body []byte
	if r.Body != nil {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			klog.Error(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		body = data
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		msg := fmt.Sprintf("contentType=%s, expected application/json", contentType)
		klog.Error(msg)
		http.Error(w, msg, http.StatusUnsupportedMediaType)
		return
	}

	klog.V(2).Infof("handling request: %s", body)

	requestedAdmissionReview, err := readAdmissionReview(body)
	if err != nil {
		msg := fmt.Sprintf("failed to read AdmissionReview from request body: %v", err)
		klog.Error(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	responseAdmissionReview := &admissionv1.AdmissionReview{}
	responseAdmissionReview.SetGroupVersionKind(requestedAdmissionReview.GroupVersionKind())
	responseAdmissionReview.Response = admit(*requestedAdmissionReview)
	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

	klog.V(2).Infof("sending response: %v", responseAdmissionReview)
	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		klog.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		klog.Error(err)
	}
}

func readAdmissionReview(data []byte) (*admissionv1.AdmissionReview, error) {
	deserializer := codecs.UniversalDeserializer()
	obj, gvk, err := deserializer.Decode(data, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("request could not be decoded: %w", err)
	}

	if *gvk != admissionv1.SchemeGroupVersion.WithKind("AdmissionReview") {
		return nil, fmt.Errorf("unsupported group version kind: %v", gvk)
	}

	requestedAdmissionReview, ok := obj.(*admissionv1.AdmissionReview)
	if !ok {
		return nil, fmt.Errorf("expected v1.AdmissionReview but got: %T", obj)
	}

	return requestedAdmissionReview, nil
}

// admitResourceClaimParameters accepts both ResourceClaims and ResourceClaimTemplates and validates their
// opaque device configuration parameters for this driver.
func admitResourceClaimParameters(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	klog.V(2).Info("admitting resource claim parameters")

	var deviceConfigs []resourceapi.DeviceClaimConfiguration
	var specPath string

	raw := ar.Request.Object.Raw
	deserializer := codecs.UniversalDeserializer()

	switch ar.Request.Resource {
	case resourceClaimResource:
		claim := resourceapi.ResourceClaim{}
		if _, _, err := deserializer.Decode(raw, nil, &claim); err != nil {
			klog.Error(err)
			return &admissionv1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
					Reason:  metav1.StatusReasonBadRequest,
				},
			}
		}
		deviceConfigs = claim.Spec.Devices.Config
		specPath = "spec"
	case resourceClaimTemplateResource:
		claimTemplate := resourceapi.ResourceClaimTemplate{}
		if _, _, err := deserializer.Decode(raw, nil, &claimTemplate); err != nil {
			klog.Error(err)
			return &admissionv1.AdmissionResponse{
				Result: &metav1.Status{
					Message: err.Error(),
					Reason:  metav1.StatusReasonBadRequest,
				},
			}
		}
		deviceConfigs = claimTemplate.Spec.Spec.Devices.Config
		specPath = "spec.spec"
	default:
		msg := fmt.Sprintf("expected resource to be %s or %s, got %s", resourceClaimResource, resourceClaimTemplateResource, ar.Request.Resource)
		klog.Error(msg)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: msg,
				Reason:  metav1.StatusReasonBadRequest,
			},
		}
	}

	var errs []error
	for configIndex, config := range deviceConfigs {
		if config.Opaque == nil || config.Opaque.Driver != DriverName {
			continue
		}

		fieldPath := fmt.Sprintf("%s.devices.config[%d].opaque.parameters", specPath, configIndex)
		decodedConfig, err := runtime.Decode(configapi.Decoder, config.DeviceConfiguration.Opaque.Parameters.Raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("error decoding object at %s: %w", fieldPath, err))
			continue
		}
		gpuConfig, ok := decodedConfig.(*configapi.GpuConfig)
		if !ok {
			errs = append(errs, fmt.Errorf("expected v1alpha1.GpuConfig at %s but got: %T", fieldPath, decodedConfig))
			continue
		}
		err = gpuConfig.Validate()
		if err != nil {
			errs = append(errs, fmt.Errorf("object at %s is invalid: %w", fieldPath, err))
		}
	}

	if len(errs) > 0 {
		var errMsgs []string
		for _, err := range errs {
			errMsgs = append(errMsgs, err.Error())
		}
		msg := fmt.Sprintf("%d configs failed to validate: %s", len(errs), strings.Join(errMsgs, "; "))
		klog.Error(msg)
		return &admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: msg,
				Reason:  metav1.StatusReason(metav1.StatusReasonInvalid),
			},
		}
	}

	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}
