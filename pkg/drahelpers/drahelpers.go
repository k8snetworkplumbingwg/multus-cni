package drahelpers

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

const DRADelegateDir = "/run/k8s.cni.cncf.io/dra"

func LoadDelegatesFromDRAFile(podName string, podNamespace string) ([]*types.DelegateNetConf, error) {

	delegatePath := fmt.Sprintf("%s/%s_%s.json", DRADelegateDir, podNamespace, podName)
	data, err := os.ReadFile(delegatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read delegate file %s: %w", delegatePath, err)
	}

	var delegates []*types.DelegateNetConf
	if err := json.Unmarshal(data, &delegates); err != nil {
		return nil, fmt.Errorf("failed to unmarshal delegate JSON: %w", err)
	}

	return delegates, nil
}
