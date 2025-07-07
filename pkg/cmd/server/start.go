package server

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/klog/v2"

	"netguard-pg-backend/internal/k8s/apiserver"
)

// NewCommandStartNetguardServer creates a new command for starting the netguard API server
func NewCommandStartNetguardServer(ctx context.Context, out, errOut io.Writer) *cobra.Command {
	opts := options.NewRecommendedOptions(
		"/registry/netguard.sgroups.io",
		nil, /* codec */
	)

	// Disable etcd for aggregated API server
	opts.Etcd = nil

	// Enable delegation for auth
	opts.Authentication.RemoteKubeConfigFileOptional = true
	opts.Authorization.RemoteKubeConfigFileOptional = true

	cmd := &cobra.Command{
		Use:   "netguard-apiserver",
		Short: "Launch a netguard API server",
		RunE: func(c *cobra.Command, args []string) error {
			klog.Info("🚀 Starting Netguard API server with correct pattern...")

			server, err := apiserver.NewServer(opts)
			if err != nil {
				return fmt.Errorf("failed to create server: %v", err)
			}

			return server.PrepareRun().Run(ctx.Done())
		},
	}

	// Add flags from recommended options
	opts.AddFlags(cmd.Flags())
	// Make Go standard flags (including klog) available to the command so users can use -v, --v etc.
	cmd.Flags().AddGoFlagSet(flag.CommandLine)

	return cmd
}
