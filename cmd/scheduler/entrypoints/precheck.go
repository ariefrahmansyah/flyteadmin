package entrypoints

import (
	"context"
	"fmt"
	"time"

	"github.com/flyteorg/flytestdlib/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/avast/retry-go"
	adminClient "github.com/flyteorg/flyteidl/clients/go/admin"
	"github.com/pkg/errors"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/spf13/cobra"
)

const (
	timeout = 30 * time.Second
)

var preCheckRunCmd = &cobra.Command{
	Use:   "precheck",
	Short: "This command will check pre requirement for scheduler",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := []grpc.DialOption{
			grpc.WithUserAgent("grpc_health_probe"),
			grpc.WithBlock(),
			grpc.WithInsecure(),
		}
		ctx := context.Background()
		config := adminClient.GetConfig(ctx)

		err := retry.Do(
			func() error {
				dialCtx, dialCancel := context.WithTimeout(ctx, timeout)
				defer dialCancel()
				conn, err := grpc.DialContext(dialCtx, config.Endpoint.String(), opts...)
				if err != nil {
					if err == context.DeadlineExceeded {
						logger.Printf(ctx, "timeout: failed to connect service %q within %v", config.Endpoint.String(), timeout)
						return fmt.Errorf("timeout: failed to connect service %q within %v", config.Endpoint.String(), timeout)
					}
					logger.Printf(ctx, "error: failed to connect service at %q: %+v", config.Endpoint.String(), err)
					return fmt.Errorf("error: failed to connect service at %q: %+v", config.Endpoint.String(), err)
				}
				rpcCtx := metadata.NewOutgoingContext(ctx, metadata.MD{})
				resp, err := healthpb.NewHealthClient(conn).Check(rpcCtx,
					&healthpb.HealthCheckRequest{
						Service: "",
					})
				if err != nil {
					if stat, ok := status.FromError(err); ok && stat.Code() == codes.Unimplemented {
						return retry.Unrecoverable(err)
					} else if stat, ok := status.FromError(err); ok && stat.Code() == codes.DeadlineExceeded {
						logger.Printf(ctx, "timeout: health rpc did not complete within %v", timeout)
						return fmt.Errorf("timeout: health rpc did not complete within %v", timeout)
					}
					return err
				}
				if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
					return errors.New("Health check failed")
				}
				return nil
			},
			retry.Delay(retry.BackOffDelay(10, nil, &retry.Config{})),
		)
		if err != nil {
			return err
		}

		logger.Printf(ctx, "Flyteadmin is up & running")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(preCheckRunCmd)
}
