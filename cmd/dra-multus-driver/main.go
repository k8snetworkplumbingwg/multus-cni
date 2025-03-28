package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/flags"
)

const (
	DriverName                 = "multus-dra.k8s.cni.cncf.io"
	PluginRegistrationPath     = "/var/lib/kubelet/plugins_registry/" + DriverName + ".sock"
	DriverPluginPath           = "/var/lib/kubelet/plugins/" + DriverName
	DriverPluginSocketPath     = DriverPluginPath + "/plugin.sock"
	DriverPluginCheckpointFile = "checkpoint.json"
)

type Flags struct {
	cdiRoot          string
	kubeClientConfig flags.KubeClientConfig
	loggingConfig    *flags.LoggingConfig
	nodeName         string
}

type Config struct {
	flags      *Flags
	coreclient coreclientset.Interface
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
			Name:        "node-name",
			Usage:       "The name of the node to be worked on.",
			Required:    true,
			Destination: &flags.nodeName,
			EnvVars:     []string{"NODE_NAME"},
		},
		&cli.StringFlag{
			Name:        "cdi-root",
			Usage:       "Absolute path to the directory where CDI files will be generated.",
			Value:       "/etc/cdi",
			Destination: &flags.cdiRoot,
			EnvVars:     []string{"CDI_ROOT"},
		},
	}
	cliFlags = append(cliFlags, flags.kubeClientConfig.Flags()...)
	cliFlags = append(cliFlags, flags.loggingConfig.Flags()...)

	app := &cli.App{
		Name:            "multus-dra-driver",
		Usage:           "Multus-integrated DRA driver for resolving NetworkAttachmentDefinitions at scheduling time",
		HideHelpCommand: true,
		Flags:           cliFlags,
		Before: func(c *cli.Context) error {
			if c.Args().Len() > 0 {
				return fmt.Errorf("arguments not supported: %v", c.Args().Slice())
			}
			return flags.loggingConfig.Apply()
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context
			clientSets, err := flags.kubeClientConfig.NewClientSets()
			if err != nil {
				return fmt.Errorf("create client: %v", err)
			}

			config := &Config{
				flags:      flags,
				coreclient: clientSets.Core,
			}

			return StartPlugin(ctx, config)
		},
	}

	return app
}

func StartPlugin(ctx context.Context, config *Config) error {
	klog.Infof("Creating driver plugin directory: %s", DriverPluginPath)
	err := os.MkdirAll(DriverPluginPath, 0750)
	if err != nil {
		return err
	}

	klog.Infof("Checking CDI root: %s", config.flags.cdiRoot)
	info, err := os.Stat(config.flags.cdiRoot)
	switch {
	case err != nil && os.IsNotExist(err):
		err := os.MkdirAll(config.flags.cdiRoot, 0750)
		if err != nil {
			return err
		}
	case err != nil:
		return err
	case !info.IsDir():
		return fmt.Errorf("path for cdi file generation is not a directory: '%v'", err)
	}

	klog.Infof("Starting %s", DriverName)
	driver, err := NewDriver(ctx, config)
	if err != nil {
		return err
	}

	// Watch for shutdown signals
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-sigc

	err = driver.Shutdown(ctx)
	if err != nil {
		klog.FromContext(ctx).Error(err, "Unable to cleanly shutdown driver")
	}

	return nil
}
