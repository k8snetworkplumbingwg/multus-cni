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

// This is a entrypoint for thin (stand-alone) images.
package main

import (
	"bytes"
	"crypto/sha256"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/containernetworking/cni/libcni"
	"github.com/spf13/pflag"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/cmdutils"
)

// Options stores command line options
type Options struct {
	CNIBinDir                string
	CNIConfDir               string
	CNIVersion               string
	MultusConfFile           string
	MultusBinFile            string // may be hidden or remove?
	MultusCNIConfDir         string
	SkipMultusBinaryCopy     bool
	MultusKubeConfigFileHost string
	MultusMasterCNIFileName  string
	NamespaceIsolation       bool
	GlobalNamespaces         string
	MultusAutoconfigDir      string
	MultusLogToStderr        bool
	MultusLogLevel           string
	MultusLogFile            string
	OverrideNetworkName      bool
	CleanupConfigOnExit      bool
	RenameConfFile           bool
	ReadinessIndicatorFile   string
	AdditionalBinDir         string
	ForceCNIVersion          bool
	SkipTLSVerify            bool
}

const (
	serviceAccountTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	serviceAccountCAFile    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

func (o *Options) addFlags() {
	pflag.ErrHelp = nil // suppress error message for help
	fs := pflag.CommandLine
	fs.StringVar(&o.CNIBinDir, "cni-bin-dir", "/host/opt/cni/bin", "CNI binary directory")
	fs.StringVar(&o.CNIConfDir, "cni-conf-dir", "/host/etc/cni/net.d", "CNI config directory")
	fs.StringVar(&o.CNIVersion, "cni-version", "", "CNI version for multus CNI config (e.g. '0.3.1')")
	fs.StringVar(&o.MultusConfFile, "multus-conf-file", "auto", "multus CNI config file")
	fs.StringVar(&o.MultusBinFile, "multus-bin-file", "/usr/src/multus-cni/bin/multus", "multus binary file path")
	fs.StringVar(&o.MultusCNIConfDir, "multus-cni-conf-dir", "/host/etc/cni/multus/net.d", "multus specific CNI config directory")
	fs.BoolVar(&o.SkipMultusBinaryCopy, "skip-multus-binary-copy", false, "skip multus binary file copy")

	fs.StringVar(&o.MultusKubeConfigFileHost, "multus-kubeconfig-file-host", "/etc/cni/net.d/multus.d/multus.kubeconfig", "kubeconfig for multus (used only with --multus-conf-file=auto)")
	fs.StringVar(&o.MultusMasterCNIFileName, "multus-master-cni-file-name", "", "master CNI file in multus-autoconfig-dir")
	fs.BoolVar(&o.NamespaceIsolation, "namespace-isolation", false, "namespace isolation")
	fs.StringVar(&o.GlobalNamespaces, "global-namespaces", "", "global namespaces, comma separated (used only with --namespace-isolation=true)")
	fs.StringVar(&o.MultusAutoconfigDir, "multus-autoconfig-dir", "/host/etc/cni/net.d", "multus autoconfig dir (used only with --multus-conf-file=auto)")
	fs.BoolVar(&o.MultusLogToStderr, "multus-log-to-stderr", true, "log to stderr")
	fs.StringVar(&o.MultusLogLevel, "multus-log-level", "", "multus log level")
	fs.StringVar(&o.MultusLogFile, "multus-log-file", "", "multus log file")
	fs.BoolVar(&o.OverrideNetworkName, "override-network-name", false, "override network name from master cni file (used only with --multus-conf-file=auto)")
	fs.BoolVar(&o.CleanupConfigOnExit, "cleanup-config-on-exit", false, "cleanup config file on exit (used only with --multus-conf-file=auto)")
	fs.BoolVar(&o.RenameConfFile, "rename-conf-file", false, "rename master config file to invalidate (used only with --multus-conf-file=auto)")
	fs.StringVar(&o.ReadinessIndicatorFile, "readiness-indicator-file", "", "readiness indicator file (used only with --multus-conf-file=auto)")
	fs.StringVar(&o.AdditionalBinDir, "additional-bin-dir", "", "adds binDir option to configuration (used only with --multus-conf-file=auto)")
	fs.BoolVar(&o.SkipTLSVerify, "skip-tls-verify", false, "skip TLS verify")
	fs.BoolVar(&o.ForceCNIVersion, "force-cni-version", false, "force cni version to '--cni-version' (only for e2e-kind testing)")
	fs.MarkHidden("force-cni-version")
	fs.MarkHidden("skip-tls-verify")
}

func (o *Options) verifyFileExists() error {
	// CNIConfDir
	if _, err := os.Stat(o.CNIConfDir); err != nil {
		return fmt.Errorf("cni-conf-dir is not found: %v", err)
	}

	// CNIBinDir
	if _, err := os.Stat(o.CNIBinDir); err != nil {
		return fmt.Errorf("cni-bin-dir is not found: %v", err)
	}

	// MultusBinFile
	if _, err := os.Stat(o.MultusBinFile); err != nil {
		return fmt.Errorf("multus-bin-file is not found: %v", err)
	}

	if o.MultusConfFile != "auto" {
		// MultusConfFile
		if _, err := os.Stat(o.MultusConfFile); err != nil {
			return fmt.Errorf("multus-conf-file is not found: %v", err)
		}
	}
	return nil
}

const kubeConfigTemplate = `# Kubeconfig file for Multus CNI plugin.
apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: {{ .KubeConfigHost }}
    {{ .KubeServerTLS }}
users:
- name: multus
  user:
    token: "{{ .KubeServiceAccountToken }}"
contexts:
- name: multus-context
  context:
    cluster: local
    user: multus
current-context: multus-context
`

func (o *Options) createKubeConfig(currentFileHash []byte) ([]byte, error) {
	// check file exists
	if _, err := os.Stat(serviceAccountTokenFile); err != nil {
		return nil, fmt.Errorf("service account token is not found: %v", err)
	}
	if _, err := os.Stat(serviceAccountCAFile); err != nil {
		return nil, fmt.Errorf("service account ca is not found: %v", err)
	}

	// create multus.d directory
	if err := os.MkdirAll(fmt.Sprintf("%s/multus.d", o.CNIConfDir), 0755); err != nil {
		return nil, fmt.Errorf("cannot create multus.d directory: %v", err)
	}

	// create multus cni conf directory
	if err := os.MkdirAll(o.MultusCNIConfDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create multus-cni-conf-dir(%s) directory: %v", o.MultusCNIConfDir, err)
	}

	// get Kubernetes service protocol/host/port
	kubeProtocol := os.Getenv("KUBERNETES_SERVICE_PROTOCOL")
	if kubeProtocol == "" {
		kubeProtocol = "https"
	}
	kubeHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	kubePort := os.Getenv("KUBERNETES_SERVICE_PORT")

	// check tlsConfig
	tlsConfig := ""
	if o.SkipTLSVerify {
		tlsConfig = "insecure-skip-tls-verify: true"
	} else {
		// create tlsConfig by service account CA file
		caFileByte, err := os.ReadFile(serviceAccountCAFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read service account ca file: %v", err)
		}
		caFileB64 := bytes.ReplaceAll([]byte(b64.StdEncoding.EncodeToString(caFileByte)), []byte("\n"), []byte(""))
		tlsConfig = fmt.Sprintf("certificate-authority-data: %s", string(caFileB64))
	}

	saTokenByte, err := os.ReadFile(serviceAccountTokenFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read service account token file: %v", err)
	}

	// create kubeconfig by template and replace it by atomic
	tempKubeConfigFile := fmt.Sprintf("%s/multus.d/multus.kubeconfig.new", o.CNIConfDir)
	multusKubeConfig := fmt.Sprintf("%s/multus.d/multus.kubeconfig", o.CNIConfDir)
	fp, err := os.OpenFile(tempKubeConfigFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot create kubeconfig temp file: %v", err)
	}

	templateKubeconfig, err := template.New("kubeconfig").Parse(kubeConfigTemplate)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %v", err)
	}
	templateData := map[string]string{
		"KubeConfigHost":          fmt.Sprintf("%s://[%s]:%s", kubeProtocol, kubeHost, kubePort),
		"KubeServerTLS":           tlsConfig,
		"KubeServiceAccountToken": string(saTokenByte),
	}

	// Prepare
	hash := sha256.New()
	writer := io.MultiWriter(hash, fp)

	// genearate kubeconfig from template
	if err = templateKubeconfig.Execute(writer, templateData); err != nil {
		return nil, fmt.Errorf("cannot create kubeconfig: %v", err)
	}

	if err := fp.Sync(); err != nil {
		os.Remove(fp.Name())
		return nil, fmt.Errorf("cannot flush kubeconfig temp file: %v", err)
	}
	if err := fp.Close(); err != nil {
		os.Remove(fp.Name())
		return nil, fmt.Errorf("cannot close kubeconfig temp file: %v", err)
	}

	newFileHash := hash.Sum(nil)
	if currentFileHash != nil && bytes.Compare(newFileHash, currentFileHash) == 0 {
		fmt.Printf("kubeconfig is same, not copy\n")
		os.Remove(fp.Name())
		return currentFileHash, nil
	}

	// replace file with tempfile
	if err := os.Rename(tempKubeConfigFile, multusKubeConfig); err != nil {
		return nil, fmt.Errorf("cannot replace %q with temp file %q: %v", multusKubeConfig, tempKubeConfigFile, err)
	}

	fmt.Printf("kubeconfig is created in %s\n", multusKubeConfig)
	return newFileHash, nil
}

const multusConflistTemplate = `{
    "cniVersion": "{{ .CNIVersion }}",
    "name": "{{ .MasterPluginNetworkName }}",
    "plugins": [ {
        "type": "multus",{{
            .NestedCapabilities
        }}{{
            .NamespaceIsolationConfig
        }}{{
            .GlobalNamespacesConfig
        }}{{
            .LogToStderrConfig
        }}{{
            .LogLevelConfig
        }}{{
            .LogFileConfig
        }}{{
            .AdditionalBinDirConfig
        }}{{
            .MultusCNIConfDirConfig
        }}{{
            .ReadinessIndicatorFileConfig
        }}
        "kubeconfig": "{{ .MultusKubeConfigFileHost }}",
        "delegates": [
            {{ .MasterPluginJSON }}
        ]
    }]
}
`

const multusConfTemplate = `{
        "cniVersion": "{{ .CNIVersion }}",
        "name": "{{ .MasterPluginNetworkName }}",
        "type": "multus",{{
            .NestedCapabilities
        }}{{
            .NamespaceIsolationConfig
        }}{{
            .GlobalNamespacesConfig
        }}{{
            .LogToStderrConfig
        }}{{
            .LogLevelConfig
        }}{{
            .LogFileConfig
        }}{{
            .AdditionalBinDirConfig
        }}{{
            .MultusCNIConfDirConfig
        }}{{
            .ReadinessIndicatorFileConfig
        }}
        "kubeconfig": "{{ .MultusKubeConfigFileHost }}",
        "delegates": [
                {{ .MasterPluginJSON }}
        ]
}
`

func (o *Options) createMultusConfig() (string, error) {
	// find master file from MultusAutoconfigDir
	files, err := libcni.ConfFiles(o.MultusAutoconfigDir, []string{".conf", ".conflist"})
	if err != nil {
		return "", fmt.Errorf("cannot find master CNI config in %q: %v", o.MultusAutoconfigDir, err)
	}

	masterConfigPath := ""
	for _, filename := range files {
		if !strings.HasPrefix(filepath.Base(filename), "00-multus.conf") {
			masterConfigPath = filename
			break
		}
	}
	if masterConfigPath == "" {
		return "", fmt.Errorf("cannot find valid master CNI config in %q", o.MultusAutoconfigDir)
	}

	masterConfigBytes, err := os.ReadFile(masterConfigPath)
	if err != nil {
		return "", fmt.Errorf("cannot read master CNI config file %q: %v", masterConfigPath, err)
	}
	masterConfig := map[string]interface{}{}
	if err = json.Unmarshal(masterConfigBytes, &masterConfig); err != nil {
		return "", fmt.Errorf("cannot read master CNI config json: %v", err)
	}

	// check CNIVersion
	masterCNIVersionElem, ok := masterConfig["cniVersion"]
	if !ok {
		return "", fmt.Errorf("cannot get cniVersion in master CNI config file %q: %v", masterConfigPath, err)
	}

	if o.ForceCNIVersion {
		masterConfig["cniVersion"] = o.CNIVersion
		fmt.Printf("force CNI version to %q\n", o.CNIVersion)
	} else {
		masterCNIVersion := masterCNIVersionElem.(string)
		if o.CNIVersion != "" && masterCNIVersion != o.CNIVersion {
			return "", fmt.Errorf("Multus cni version is %q while master plugin cni version is %q", o.CNIVersion, masterCNIVersion)
		}
		o.CNIVersion = masterCNIVersion
	}
	cniVersionConfig := o.CNIVersion

	// check OverrideNetworkName (if true, get master plugin name, otherwise 'multus-cni-network'
	masterPluginNetworkName := "multus-cni-network"
	if o.OverrideNetworkName {
		masterPluginNetworkElem, ok := masterConfig["name"]
		if !ok {
			return "", fmt.Errorf("cannot get name in master CNI config file %q: %v", masterConfigPath, err)
		}

		masterPluginNetworkName = masterPluginNetworkElem.(string)
		fmt.Printf("master plugin name is overrided to %q\n", masterPluginNetworkName)
	}

	// check capabilities (from master conf, top and 'plugins')
	masterCapabilities := map[string]bool{}
	_, isMasterConfList := masterConfig["plugins"]

	if isMasterConfList {
		masterPluginsElem, ok := masterConfig["plugins"]
		if !ok {
			return "", fmt.Errorf("cannot get 'plugins' field in master CNI config file %q: %v", masterConfigPath, err)
		}
		masterPlugins := masterPluginsElem.([]interface{})
		for _, v := range masterPlugins {
			pluginFields := v.(map[string]interface{})
			capabilitiesElem, ok := pluginFields["capabilities"]
			if ok {
				capabilities := capabilitiesElem.(map[string]interface{})
				for k, v := range capabilities {
					masterCapabilities[k] = v.(bool)
				}
			}
		}
		fmt.Printf("master capabilities is get from conflist\n")
	} else {
		masterCapabilitiesElem, ok := masterConfig["capabilities"]
		if ok {
			for k, v := range masterCapabilitiesElem.(map[string]interface{}) {
				masterCapabilities[k] = v.(bool)
			}
		}
		fmt.Printf("master capabilities is get from conffile\n")
	}
	nestedCapabilitiesConf := ""
	if len(masterCapabilities) != 0 {
		capabilitiesByte, err := json.Marshal(masterCapabilities)
		if err != nil {
			return "", fmt.Errorf("cannot get capabilities map: %v", err)
		}
		nestedCapabilitiesConf = fmt.Sprintf("\n        \"capabilities\": %s,", string(capabilitiesByte))
	}

	// check NamespaceIsolation
	namespaceIsolationConfig := ""
	if o.NamespaceIsolation {
		namespaceIsolationConfig = "\n        \"namespaceIsolation\": true,"
	}

	// check GlobalNamespaces
	globalNamespaceConfig := ""
	if o.GlobalNamespaces != "" {
		globalNamespaceConfig = fmt.Sprintf("\n        \"globalNamespaces\": %q,", o.GlobalNamespaces)
	}

	// check MultusLogToStderr
	logToStderrConfig := ""
	if !o.MultusLogToStderr {
		logToStderrConfig = "\n        \"logToStderr\": false,"
	}

	// check MultusLogLevel (debug/error/panic/verbose) and reject others
	logLevelConfig := ""
	logLevelStr := strings.ToLower(o.MultusLogLevel)
	switch logLevelStr {
	case "debug", "error", "panic", "verbose":
		logLevelConfig = fmt.Sprintf("\n        \"logLevel\": %q,", logLevelStr)
	case "":
		// no logLevel config, skipped
	default:
		return "", fmt.Errorf("Log levels should be one of: debug/verbose/error/panic, did not understand: %q", o.MultusLogLevel)
	}

	// check MultusLogFile
	logFileConfig := ""
	if o.MultusLogFile != "" {
		logFileConfig = fmt.Sprintf("\n        \"logFile\": %q,", o.MultusLogFile)
	}

	// check AdditionalBinDir
	additionalBinDirConfig := ""
	if o.AdditionalBinDir != "" {
		additionalBinDirConfig = fmt.Sprintf("\n        \"binDir\": %q,", o.AdditionalBinDir)
	}

	// check MultusCNIConfDir
	multusCNIConfDirConfig := ""
	if o.MultusCNIConfDir != "" {
		multusCNIConfDirConfig = fmt.Sprintf("\n        \"cniConf\": %q,", o.MultusCNIConfDir)
	}

	// check ReadinessIndicatorFile
	readinessIndicatorFileConfig := ""
	if o.ReadinessIndicatorFile != "" {
		readinessIndicatorFileConfig = fmt.Sprintf("\n        \"readinessindicatorfile\": %q,", o.ReadinessIndicatorFile)
	}

	// fill .MasterPluginJSON
	masterPluginByte, err := json.Marshal(masterConfig)
	if err != nil {
		return "", fmt.Errorf("cannot encode master CNI config: %v", err)
	}

	// generate multus config
	tempFileName := fmt.Sprintf("%s/00-multus.conf.new", o.CNIConfDir)
	fp, err := os.OpenFile(tempFileName, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return "", fmt.Errorf("cannot create multus cni temp file: %v", err)
	}

	// use conflist template if cniVersionConfig == "1.0.0"
	multusConfFilePath := fmt.Sprintf("%s/00-multus.conf", o.CNIConfDir)
	templateMultusConfig, err := template.New("multusCNIConfig").Parse(multusConfTemplate)
	if err != nil {
		return "", fmt.Errorf("template parse error: %v", err)
	}

	if o.CNIVersion == "1.0.0" { //Check 1.0.0 or above!
		multusConfFilePath = fmt.Sprintf("%s/00-multus.conflist", o.CNIConfDir)
		templateMultusConfig, err = template.New("multusCNIConfig").Parse(multusConflistTemplate)
		if err != nil {
			return "", fmt.Errorf("template parse error: %v", err)
		}
	}

	templateData := map[string]string{
		"CNIVersion":                   cniVersionConfig,
		"MasterPluginNetworkName":      masterPluginNetworkName,
		"NestedCapabilities":           nestedCapabilitiesConf,
		"NamespaceIsolationConfig":     namespaceIsolationConfig,
		"GlobalNamespacesConfig":       globalNamespaceConfig,
		"LogToStderrConfig":            logToStderrConfig,
		"LogLevelConfig":               logLevelConfig,
		"LogFileConfig":                logFileConfig,
		"AdditionalBinDirConfig":       additionalBinDirConfig,
		"MultusCNIConfDirConfig":       multusCNIConfDirConfig,
		"ReadinessIndicatorFileConfig": readinessIndicatorFileConfig,
		"MultusKubeConfigFileHost":     o.MultusKubeConfigFileHost, // be fixed?
		"MasterPluginJSON":             string(masterPluginByte),
	}
	if err = templateMultusConfig.Execute(fp, templateData); err != nil {
		return "", fmt.Errorf("cannot create multus cni config: %v", err)
	}

	if err := fp.Sync(); err != nil {
		os.Remove(tempFileName)
		return "", fmt.Errorf("cannot flush multus cni config: %v", err)
	}
	if err := fp.Close(); err != nil {
		os.Remove(tempFileName)
		return "", fmt.Errorf("cannot close multus cni config: %v", err)
	}

	if err := os.Rename(tempFileName, multusConfFilePath); err != nil {
		return "", fmt.Errorf("cannot replace %q with temp file %q: %v", multusConfFilePath, tempFileName, err)
	}

	if o.RenameConfFile {
		//masterConfigPath
		renamedMasterConfigPath := fmt.Sprintf("%s.old", masterConfigPath)
		if err := os.Rename(masterConfigPath, renamedMasterConfigPath); err != nil {
			return "", fmt.Errorf("cannot move original master file to %q", renamedMasterConfigPath)
		}
		fmt.Printf("Original master file moved to %q\n", renamedMasterConfigPath)
	}

	return masterConfigPath, nil
}

func main() {
	opt := Options{}
	opt.addFlags()
	helpFlag := pflag.BoolP("help", "h", false, "show help message and quit")

	pflag.Parse()
	if *helpFlag {
		pflag.PrintDefaults()
		os.Exit(1)
	}

	err := opt.verifyFileExists()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	// copy multus binary
	if !opt.SkipMultusBinaryCopy {
		// Copy
		if err = cmdutils.CopyFileAtomic(opt.MultusBinFile, opt.CNIBinDir, "_multus", "multus"); err != nil {
			fmt.Fprintf(os.Stderr, "failed at multus copy: %v\n", err)
			return
		}
	}

	var kubeConfigHash []byte
	var masterConfigFilePath string
	// copy user specified multus conf to CNI conf directory
	if opt.MultusConfFile != "auto" {
		confFileName := filepath.Base(opt.MultusConfFile)
		tempConfFileName := fmt.Sprintf("%s.temp", confFileName)
		if err = cmdutils.CopyFileAtomic(opt.MultusConfFile, opt.CNIConfDir, tempConfFileName, confFileName); err != nil {
			fmt.Fprintf(os.Stderr, "failed at copy multus conf file: %v\n", err)
			return
		}
		fmt.Printf("multus config file %s is copied.\n", opt.MultusConfFile)
	} else { // auto generate multus config
		kubeConfigHash, err = opt.createKubeConfig(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create multus kubeconfig: %v\n", err)
			return
		}
		fmt.Printf("kubeconfig file is created.\n")
		masterConfigFilePath, err = opt.createMultusConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create multus config: %v\n", err)
			return
		}
		fmt.Printf("multus config file is created.\n")
	}

	if opt.CleanupConfigOnExit && opt.MultusConfFile == "auto" {
		fmt.Printf("Entering watch loop...\n")
		for {
			// Check kubeconfig and update if different (i.e. service account updated)
			kubeConfigHash, err = opt.createKubeConfig(kubeConfigHash)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to update multus kubeconfig: %v\n", err)
				return
			}

			// TODO: should we watch master CNI config (by fsnotify? https://github.com/fsnotify/fsnotify)
			_, err = os.Stat(masterConfigFilePath)

			// if masterConfigFilePath is no longer exists
			if os.IsNotExist(err) {
				fmt.Printf("Master plugin @ %q has been deleted. Allowing 45 seconds for its restoration...\n", masterConfigFilePath)
				time.Sleep(10 * time.Second)

				for range time.Tick(1 * time.Second) {
					_, err = os.Stat(masterConfigFilePath)
					if !os.IsNotExist(err) {
						fmt.Printf("Master plugin @ %q was restored. Regenerating given configuration.\n", masterConfigFilePath)
						break
					}
				}
			}
			masterConfigFilePath, err = opt.createMultusConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create multus config: %v\n", err)
				return
			}
			time.Sleep(1 * time.Second)
		}
	} else {
		// sleep infinitely
		for {
			time.Sleep(time.Duration(1<<63 - 1))
		}
	}
}
