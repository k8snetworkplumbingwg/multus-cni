// Copyright (c) 2023 Network Plumbing Working Group
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

// This is Kubernetes controller which approves CSR submitted by multus.
// This command is required only if multus runs with per-node certificate.
package main

// Note: cert-approver should be simple, just approve multus' CSR, hence
// this go code should not have any dependencies from pkg/, if possible,
// to keep its code simplicity.
import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/wait"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/certificate/csr"
	"k8s.io/client-go/util/workqueue"
)

// CertController object
type CertController struct {
	clientset          kubernetes.Interface
	queue              workqueue.RateLimitingInterface
	informer           cache.SharedIndexInformer
	broadcaster        record.EventBroadcaster
	recorder           record.EventRecorder
	commonNamePrefixes string
}

const (
	maxDuration                = time.Hour * 24 * 365
	resyncPeriod time.Duration = time.Second * 3600 // resync every one hour, default is 10 hour
	maxRetries                 = 5
)

var (
	ControllerName = "csr-approver"
	NamePrefix     = "system:multus"
	Organization   = []string{"system:multus"}
	Groups         = sets.New[string]("system:nodes", "system:multus", "system:authenticated")
	UserPrefixes   = sets.New[string]("system:node", NamePrefix)
	Usages         = sets.New[certificatesv1.KeyUsage](
		certificatesv1.UsageDigitalSignature,
		certificatesv1.UsageClientAuth)
)

func NewCertController() (*CertController, error) {
	var clientset kubernetes.Interface
	/* setup Kubernetes API client */
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	informer := cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(
			clientset.CertificatesV1().RESTClient(),
			"certificatesigningrequests", corev1.NamespaceAll, fields.Everything()),
		&certificatesv1.CertificateSigningRequest{},
		resyncPeriod,
		nil)

	broadcaster := record.NewBroadcaster()
	broadcaster.StartLogging(klog.Infof)
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientset.CoreV1().Events("")})
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "cert-approver"})
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	c := &CertController{
		clientset:          clientset,
		informer:           informer,
		queue:              queue,
		commonNamePrefixes: NamePrefix,
		broadcaster:        broadcaster,
		recorder:           recorder,
	}

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if csr, ok := obj.(*certificatesv1.CertificateSigningRequest); ok {
				if c.filterCSR(csr) {
					key, err := cache.MetaNamespaceKeyFunc(obj)
					if err == nil {
						queue.Add(key)
					}
				}
			}
		},
	})

	return c, nil
}

func (c *CertController) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Info("Starting cert approver")

	go c.informer.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	klog.Info("cert approver synced and ready")
	wait.Until(c.runWorker, time.Second, stopCh)
}

// HasSynced is required for the cache.Controller interface.
func (c *CertController) HasSynced() bool {
	return c.informer.HasSynced()
}

// LastSyncResourceVersion is required for the cache.Controller interface.
func (c *CertController) LastSyncResourceVersion() string {
	return c.informer.LastSyncResourceVersion()
}

func (c *CertController) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *CertController) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)

	// Invoke the method containing the business logic
	err := c.processItem(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)
	return true

}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *CertController) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < maxRetries {
		klog.Infof("Error syncing csr %s: %v", key, err)
		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	utilruntime.HandleError(err)
	klog.Infof("Dropping csr %q out of the queue: %v", key, err)
}

func (c *CertController) processItem(key string) error {
	startTime := time.Now()

	obj, _, err := c.informer.GetIndexer().GetByKey(key)
	if err != nil {
		return fmt.Errorf("Error fetching object with key %s from store: %v", key, err)
	}

	req, _ := obj.(*certificatesv1.CertificateSigningRequest)

	nodeName := "unknown"
	defer func() {
		klog.Infof("Finished syncing CSR %s for %s node in %v", req.Name, nodeName, time.Since(startTime))
	}()

	if len(req.Status.Certificate) > 0 {
		klog.V(5).Infof("CSR %s is already signed", req.Name)
		return nil
	}

	if isApprovedOrDenied(&req.Status) {
		klog.V(5).Infof("CSR %s is already approved/denied", req.Name)
		return nil
	}

	csrPEM, _ := pem.Decode(req.Spec.Request)
	if csrPEM == nil {
		return fmt.Errorf("failed to PEM-parse the CSR block in .spec.request: no CSRs were found")
	}

	x509CSR, err := x509.ParseCertificateRequest(csrPEM.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse the CSR bytes: %v", err)
	}

	i := strings.LastIndex(req.Spec.Username, ":")
	if i == -1 || i == len(req.Spec.Username)-1 {
		return fmt.Errorf("failed to parse the username: %s", req.Spec.Username)
	}

	ctx := context.Background()
	prefix := req.Spec.Username[:i]
	nodeName = req.Spec.Username[i+1:]
	if !UserPrefixes.Has(prefix) {
		return c.denyCSR(ctx, req, fmt.Sprintf("CSR %q was created by an unexpected user: %q", req.Name, req.Spec.Username))
	}

	if errs := validation.IsDNS1123Subdomain(nodeName); len(errs) != 0 {
		return c.denyCSR(ctx, req, fmt.Sprintf("extracted node name %q is not a valid DNS subdomain %v", nodeName, errs))
	}

	if usages := sets.New[certificatesv1.KeyUsage](req.Spec.Usages...); !usages.Equal(Usages) {
		return c.denyCSR(ctx, req, fmt.Sprintf("CSR %q was created with unexpected usages: %v", req.Name, usages.UnsortedList()))
	}

	if !Groups.HasAll(req.Spec.Groups...) {
		return c.denyCSR(ctx, req, fmt.Sprintf("CSR %q was created by a user with unexpected groups: %v", req.Name, req.Spec.Groups))
	}

	expectedSubject := fmt.Sprintf("%s:%s", c.commonNamePrefixes, nodeName)
	if x509CSR.Subject.CommonName != expectedSubject {
		return c.denyCSR(ctx, req, fmt.Sprintf("expected the CSR's commonName to be %q, but it is %q", expectedSubject, x509CSR.Subject.CommonName))
	}

	if !reflect.DeepEqual(x509CSR.Subject.Organization, Organization) {
		return c.denyCSR(ctx, req, fmt.Sprintf("expected the CSR's organization to be %v, but it is %v", Organization, x509CSR.Subject.Organization))
	}

	if req.Spec.ExpirationSeconds == nil {
		return c.denyCSR(ctx, req, fmt.Sprintf("CSR %q was created without specyfying the expirationSeconds", req.Name))
	}

	if csr.ExpirationSecondsToDuration(*req.Spec.ExpirationSeconds) > maxDuration {
		return c.denyCSR(ctx, req, fmt.Sprintf("CSR %q was created with invalid expirationSeconds value: %d", req.Name, *req.Spec.ExpirationSeconds))
	}

	return c.approveCSR(ctx, req)
}

// CSR specific functions

func (c *CertController) filterCSR(csr *certificatesv1.CertificateSigningRequest) bool {
	nsName := types.NamespacedName{Namespace: csr.Namespace, Name: csr.Name}
	csrPEM, _ := pem.Decode(csr.Spec.Request)
	if csrPEM == nil {
		klog.Errorf("Failed to PEM-parse the CSR block in .spec.request: no CSRs were found in %s", nsName)
		return false
	}

	x509CSR, err := x509.ParseCertificateRequest(csrPEM.Bytes)
	if err != nil {
		klog.Errorf("Failed to parse the CSR .spec.request of %q: %v", nsName, err)
		return false
	}

	return strings.HasPrefix(x509CSR.Subject.CommonName, c.commonNamePrefixes) &&
		csr.Spec.SignerName == certificatesv1.KubeAPIServerClientSignerName
}

func (c *CertController) approveCSR(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) error {
	csr.Status.Conditions = append(csr.Status.Conditions,
		certificatesv1.CertificateSigningRequestCondition{
			Type:    certificatesv1.CertificateApproved,
			Status:  corev1.ConditionTrue,
			Reason:  "AutoApproved",
			Message: fmt.Sprintf("Auto-approved CSR %q", csr.Name),
		})

	c.recorder.Eventf(csr, corev1.EventTypeNormal, "CSRApproved", "CSR %q has been approved by %s", csr.Name, ControllerName)
	_, err := c.clientset.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, csr.Name, csr, metav1.UpdateOptions{})
	return err
}

func (c *CertController) denyCSR(ctx context.Context, csr *certificatesv1.CertificateSigningRequest, message string) error {
	csr.Status.Conditions = append(csr.Status.Conditions,
		certificatesv1.CertificateSigningRequestCondition{
			Type:    certificatesv1.CertificateDenied,
			Status:  corev1.ConditionTrue,
			Reason:  "CSRDenied",
			Message: message,
		},
	)

	c.recorder.Eventf(csr, corev1.EventTypeWarning, "CSRDenied", "The CSR %q has been denied by: %s", csr.Name, ControllerName, message)
	_, err := c.clientset.CertificatesV1().CertificateSigningRequests().Update(ctx, csr, metav1.UpdateOptions{})
	return err
}

func isApprovedOrDenied(status *certificatesv1.CertificateSigningRequestStatus) bool {
	for _, c := range status.Conditions {
		if c.Type == certificatesv1.CertificateApproved || c.Type == certificatesv1.CertificateDenied {
			return true
		}
	}
	return false
}

func main() {
	klog.Infof("starting cert-approver")

	//Start watching for pod creations
	certController, err := NewCertController()
	if err != nil {
		klog.Fatal(err)
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	go certController.Run(stopCh)

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	<-sigterm
}
