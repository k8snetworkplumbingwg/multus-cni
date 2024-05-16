// Copyright (c) 2022 Multus Authors
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

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	utilwait "k8s.io/apimachinery/pkg/util/wait"
)

const (
	// APIReadyPollDuration specifies duration for API readiness check polling
	APIReadyPollDuration = 100 * time.Millisecond
	// APIReadyPollTimeout specifies timeout for API readiness check polling
	APIReadyPollTimeout = 60000 * time.Millisecond

	// MultusCNIAPIEndpoint is an endpoint for multus CNI request (for multus-shim)
	MultusCNIAPIEndpoint = "/cni"
	// MultusDelegateAPIEndpoint is an endpoint for multus delegate request (for hotplug)
	MultusDelegateAPIEndpoint = "/delegate"
	defaultMultusRunDir       = "/run/multus/"

	// MultusHealthAPIEndpoint is an endpoint API clients can query to know if they can communicate w/ multus server
	MultusHealthAPIEndpoint = "/healthz"
)

// DoCNI sends a CNI request to the CNI server via JSON + HTTP over a root-owned unix socket,
// and returns the result
func DoCNI(url string, req interface{}, socketPath string) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CNI request %v: %v", req, err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(_, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to send CNI request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read CNI result: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CNI request failed with status %v: '%s'", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetAPIEndpoint returns endpoint URL for multus-daemon
func GetAPIEndpoint(endpoint string) string {
	return fmt.Sprintf("http://dummy%s", endpoint)
}

// CreateDelegateRequest creates Request for delegate API request
func CreateDelegateRequest(cniCommand, cniContainerID, cniNetNS, cniIFName, podNamespace, podName, podUID string, cniConfig []byte, interfaceAttributes *DelegateInterfaceAttributes) *Request {
	return &Request{
		Env: map[string]string{
			"CNI_COMMAND":     strings.ToUpper(cniCommand),
			"CNI_CONTAINERID": cniContainerID,
			"CNI_NETNS":       cniNetNS,
			"CNI_IFNAME":      cniIFName,
			"CNI_ARGS":        fmt.Sprintf("K8S_POD_NAMESPACE=%s;K8S_POD_NAME=%s;K8S_POD_UID=%s", podNamespace, podName, podUID),
		},
		Config:              cniConfig,
		InterfaceAttributes: interfaceAttributes,
	}
}

// WaitUntilAPIReady checks API readiness
func WaitUntilAPIReady(socketPath string) error {
	return utilwait.PollImmediate(APIReadyPollDuration, APIReadyPollTimeout, func() (bool, error) {
		_, err := DoCNI(GetAPIEndpoint(MultusHealthAPIEndpoint), nil, SocketPath(socketPath))
		return err == nil, nil
	})
}

// CheckAPIReadyNow checks API readiness once
func CheckAPIReadyNow(socketPath string) error {
	_, err := DoCNI(GetAPIEndpoint(MultusHealthAPIEndpoint), nil, SocketPath(socketPath))
	if err != nil {
		return fmt.Errorf("CheckAPIReadyNow: Daemon not reachable over socketfile: %v", err)
	}
	return nil
}
