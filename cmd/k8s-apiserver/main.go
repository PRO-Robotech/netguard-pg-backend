package main

import (
	"os"

	genericserver "k8s.io/apiserver/pkg/server"
	"k8s.io/klog/v2"

	command "netguard-pg-backend/pkg/cmd/server"
)

func main() {
	klog.InitFlags(nil)

	ctx := genericserver.SetupSignalContext()
	cmd := command.NewCommandStartNetguardServer(ctx, os.Stdout, os.Stderr)

	if err := cmd.ExecuteContext(ctx); err != nil {
		klog.Fatalf("command failed: %v", err)
	}
}
