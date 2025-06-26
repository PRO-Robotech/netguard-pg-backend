package main

import (
	"flag"
	"os"

	"github.com/spf13/pflag"
	"k8s.io/component-base/logs"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/k8s/apiserver"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	netguardscheme "netguard-pg-backend/pkg/k8s/clientset/versioned/scheme"
)

func init() {
	// Встроенные типы
	utilruntime.Must(clientgoscheme.AddToScheme(clientgoscheme.Scheme))

	utilruntime.Must(netguardscheme.AddToScheme(clientgoscheme.Scheme))
}

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()

	options := apiserver.NewOptions(os.Stdout, os.Stderr)
	fs := pflag.NewFlagSet("netguard-apiserver", pflag.ExitOnError)
	options.AddFlags(fs)

	klog.InitFlags(nil)
	fs.AddGoFlagSet(flag.CommandLine)

	pflag.CommandLine.AddFlagSet(fs)
	pflag.Parse()

	if err := options.Validate(); err != nil {
		klog.Fatalf("Error validating options: %v", err)
	}

	config, err := options.Config()
	if err != nil {
		klog.Fatalf("Error creating server config: %v", err)
	}

	server, err := config.Complete().New()
	if err != nil {
		klog.Fatalf("Error creating server: %v", err)
	}

	if err := server.GenericAPIServer.PrepareRun().Run(server.SetupSignalHandler()); err != nil {
		klog.Fatalf("Error running server: %v", err)
	}
}
