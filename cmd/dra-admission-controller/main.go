package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/urfave/cli/v2"

	admissionv1 "k8s.io/api/admission/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/flags"
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
	serve(w, r, admitOrMutatePod)
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

func admitOrMutatePod(ar admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	if ar.Request.Kind.Kind != "Pod" {
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	var pod corev1.Pod
	if err := json.Unmarshal(ar.Request.Object.Raw, &pod); err != nil {
		klog.Error(err)
		return toAdmissionError(err)
	}

	// Bail early if pod has DRA-style claims (skip mutation)
	for _, claim := range pod.Spec.ResourceClaims {
		if claim.Source.ResourceClaimTemplateName != nil {
			klog.Infof("Pod %s/%s uses DRA, skipping", pod.Namespace, pod.Name)
			return &admissionv1.AdmissionResponse{Allowed: true}
		}
	}

	// Parse NADs from annotations
	nadSelector := pod.Annotations["k8s.v1.cni.cncf.io/networks"]
	if nadSelector == "" {
		klog.Infof("No NAD annotation found for pod %s/%s", pod.Namespace, pod.Name)
		return &admissionv1.AdmissionResponse{Allowed: true}
	}

	// TODO: resolve NADs into delegate configs (e.g. using your existing logic)
	// TODO: generate network-status JSON

	// Create a patch to mutate the pod
	patch := []map[string]interface{}{
		{
			"op":    "add",
			"path":  "/metadata/annotations/k8s.v1.cni.cncf.io~1network-status",
			"value": `<computed-network-status-json>`,
		},
	}

	patchBytes, _ := json.Marshal(patch)
	return &admissionv1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionv1.PatchType {
			pt := admissionv1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}
