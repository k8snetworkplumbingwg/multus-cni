package cni

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v3/pkg/logging"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
)

// CmdAdd implements the CNI spec ADD command handler
func (p *Plugin) CmdAdd(args *skel.CmdArgs) error {
	body, err := p.DoCNI("http://dummy/", newCNIRequest(args))
	if err != nil {
		return err
	}

	response := &Response{}
	if err = json.Unmarshal(body, response); err != nil {
		err = fmt.Errorf("failed to unmarshal response '%s': %v", string(body), err)
		return err
	}

	logging.Verbosef("CmdAdd (shim): %s", string(body))
	return cnitypes.PrintResult(response.Result, response.Result.CNIVersion)
}

// CmdCheck implements the CNI spec CHECK command handler
func (p *Plugin) CmdCheck(args *skel.CmdArgs) error {
	body, err := p.DoCNI("http://dummy/", newCNIRequest(args))
	if err != nil {
		return err
	}

	response := &Response{}
	if err = json.Unmarshal(body, response); err != nil {
		err = fmt.Errorf("failed to unmarshal response '%s': %v", string(body), err)
		return err
	}

	logging.Verbosef("CmdAdd (shim): %s", string(body))
	return cnitypes.PrintResult(response.Result, response.Result.CNIVersion)
}

// CmdDel implements the CNI spec DEL command handler
func (p *Plugin) CmdDel(args *skel.CmdArgs) error {
	body, err := p.DoCNI("http://dummy/", newCNIRequest(args))
	if err != nil {
		return err
	}

	response := &Response{}
	if err = json.Unmarshal(body, response); err != nil {
		err = fmt.Errorf("failed to unmarshal response '%s': %v", string(body), err)
		return err
	}

	logging.Verbosef("CmdAdd (shim): %s", string(body))
	return cnitypes.PrintResult(response.Result, response.Result.CNIVersion)
}

// Create and fill a Request with this Plugin's environment and stdin which
// contain the CNI variables and configuration
func newCNIRequest(args *skel.CmdArgs) *Request {
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
	}
}

// DoCNI sends a CNI request to the CNI server via JSON + HTTP over a root-owned unix socket,
// and returns the result
func (p *Plugin) DoCNI(url string, req interface{}) ([]byte, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal CNI request %v: %v", req, err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(proto, addr string) (net.Conn, error) {
				return net.Dial("unix", p.SocketPath)
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
