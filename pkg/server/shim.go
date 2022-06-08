package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/types"
)

const (
	defaultMultusRunDir = "/run/multus/"
)

// CmdAdd implements the CNI spec ADD command handler
func CmdAdd(args *skel.CmdArgs) error {
	response, cniVersion, err := postRequest(args)
	if err != nil {
		logging.Errorf("CmdAdd (shim): %v", err)
		return err
	}

	logging.Verbosef("CmdAdd (shim): %v", *response.Result)
	return cnitypes.PrintResult(response.Result, cniVersion)
}

// CmdCheck implements the CNI spec CHECK command handler
func CmdCheck(args *skel.CmdArgs) error {
	_, _, err := postRequest(args)
	if err != nil {
		logging.Errorf("CmdCheck (shim): %v", err)
		return err
	}

	return err
}

// CmdDel implements the CNI spec DEL command handler
func CmdDel(args *skel.CmdArgs) error {
	_, _, err := postRequest(args)
	if err != nil {
		logging.Errorf("CmdDel (shim): %v", err)
		return nil
	}

	return nil
}

func postRequest(args *skel.CmdArgs) (*Response, string, error) {
	multusShimConfig, err := shimConfig(args.StdinData)
	if err != nil {
		return nil, "", fmt.Errorf("invalid CNI configuration passed to multus-shim: %w", err)
	}

	cniRequest, err := newCNIRequest(args)
	if err != nil {
		return nil, multusShimConfig.CNIVersion, err
	}

	body, err := DoCNI("http://dummy/", cniRequest, SocketPath(multusShimConfig.MultusSocketDir))
	if err != nil {
		return nil, multusShimConfig.CNIVersion, err
	}

	response := &Response{}
	if len(body) != 0 {
		if err = json.Unmarshal(body, response); err != nil {
			err = fmt.Errorf("failed to unmarshal response '%s': %v", string(body), err)
			return nil, multusShimConfig.CNIVersion, err
		}
	}
	return response, multusShimConfig.CNIVersion, nil
}

// Create and fill a Request with this Plugin's environment and stdin which
// contain the CNI variables and configuration
func newCNIRequest(args *skel.CmdArgs) (*Request, error) {
	envMap := make(map[string]string)
	for _, item := range os.Environ() {
		idx := strings.Index(item, "=")
		if idx > 0 {
			envMap[strings.TrimSpace(item[:idx])] = item[idx+1:]
		}
	}

	return &Request{
		Env:    envMap,
		Config: args.StdinData,
	}, nil
}

func shimConfig(cniConfig []byte) (*types.ShimNetConf, error) {
	multusConfig := &types.ShimNetConf{}
	if err := json.Unmarshal(cniConfig, multusConfig); err != nil {
		return nil, fmt.Errorf("failed to gather the multus configuration: %w", err)
	}
	if multusConfig.MultusSocketDir == "" {
		multusConfig.MultusSocketDir = defaultMultusRunDir
	}
	// Logging
	logging.SetLogStderr(multusConfig.LogToStderr)
	if multusConfig.LogFile != "" {
		logging.SetLogFile(multusConfig.LogFile)
	}
	if multusConfig.LogLevel != "" {
		logging.SetLogLevel(multusConfig.LogLevel)
	}
	return multusConfig, nil
}

// DoCNI sends a CNI request to the CNI server via JSON + HTTP over a root-owned unix socket,
// and returns the result
func DoCNI(url string, req interface{}, socketPath string) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CNI request %v: %v", req, err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(proto, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to send CNI request: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read CNI result: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CNI request failed with status %v: '%s'", resp.StatusCode, string(body))
	}

	return body, nil
}
