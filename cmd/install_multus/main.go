// Copyright (c) 2023 Multus Authors
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

// This is a install tool for multus plugins
package main

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/cmdutils"
)

func main() {
	typeFlag := pflag.StringP("type", "t", "", "specify installer type (thick/thin)")
	destDir := pflag.StringP("dest-dir", "d", "/host/opt/cni/bin", "destination directory")
	helpFlag := pflag.BoolP("help", "h", false, "show help message and quit")

	pflag.Parse()
	if *helpFlag {
		pflag.PrintDefaults()
		os.Exit(1)
	}

	multusFileName := ""
	switch *typeFlag {
	case "thick":
		multusFileName = "multus-shim"
	case "thin":
		multusFileName = "multus"
	default:
		fmt.Fprintf(os.Stderr, "--type is missing or --type has invalid value\n")
		os.Exit(1)
	}

	err := cmdutils.CopyFileAtomic(fmt.Sprintf("/usr/src/multus-cni/bin/%s", multusFileName), *destDir, fmt.Sprintf("%s.temp", multusFileName), multusFileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to copy file %s: %v\n", multusFileName, err)
		os.Exit(1)
	}

	fmt.Printf("multus %s copy succeeded!\n", multusFileName)

	// Copy the passthru CNI
	passthruPath := "/usr/src/multus-cni/bin/passthru"
	err = cmdutils.CopyFileAtomic(passthruPath, *destDir, fmt.Sprintf("%s.temp", "passthru"), "passthru")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to copy file %s: %v\n", multusFileName, err)
		os.Exit(1)
	}

	fmt.Printf("passthru cni %s copy succeeded!\n", passthruPath)

}
