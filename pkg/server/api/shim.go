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
//

package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
)

// ShimNetConf for the SHIM cni config file written in json
type ShimNetConf struct {
	// Note: This struct contains NetConf in pkg/types, but this struct is only used to parse
	// following fields, so we skip to include NetConf here. Other fields are directly send to
	// multus-daemon as a part of skel.CmdArgs, StdinData.
	// types.NetConf

	CNIVersion      string `json:"cniVersion,omitempty"`
	MultusSocketDir string `json:"socketDir"`
	LogFile         string `json:"logFile,omitempty"`
	LogLevel        string `json:"logLevel,omitempty"`
	LogToStderr     bool   `json:"logToStderr,omitempty"`
}

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

	body, err := DoCNI("http://dummy/cni", cniRequest, SocketPath(multusShimConfig.MultusSocketDir))
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

func shimConfig(cniConfig []byte) (*ShimNetConf, error) {
	multusConfig := &ShimNetConf{}
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
