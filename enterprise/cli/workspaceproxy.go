//go:build !slim

package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime/pprof"
	"time"

	"github.com/coreos/go-systemd/daemon"

	"github.com/coder/coder/cli/cliui"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/enterprise/wsproxy"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) workspaceProxy() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:     "workspace-proxy",
		Short:   "Manage workspace proxies",
		Aliases: []string{"proxy"},
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.proxyServer(),
			r.registerProxy(),
		},
	}

	return cmd
}

func (r *RootCmd) registerProxy() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "register",
		Short: "Register a workspace proxy",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(i *clibase.Invocation) error {
			ctx := i.Context()
			name := i.Args[0]
			// TODO: Fix all this
			resp, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
				Name:             name,
				DisplayName:      name,
				Icon:             "whocares.png",
				URL:              "http://localhost:6005",
				WildcardHostname: "",
			})
			if err != nil {
				return xerrors.Errorf("create workspace proxy: %w", err)
			}

			fmt.Println(resp.ProxyToken)
			return nil
		},
	}
	return cmd
}

func (r *RootCmd) proxyServer() *clibase.Cmd {
	var (
		// TODO: Remove options that we do not need
		cfg  = new(codersdk.DeploymentValues)
		opts = cfg.Options()
	)
	var _ = opts

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:     "server",
		Short:   "Start a workspace proxy server",
		Options: opts,
		Middleware: clibase.Chain(
			cli.WriteConfigMW(cfg),
			cli.PrintDeprecatedOptions(),
			clibase.RequireNArgs(0),
			// We need a client to connect with the primary coderd instance.
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			scd, err := cli.SetupServerCmd(inv, cfg)
			if err != nil {
				return err
			}
			defer scd.Close()
			ctx := scd.Ctx

			pu, _ := url.Parse("http://localhost:3000")
			proxy, err := wsproxy.New(&wsproxy.Options{
				Logger: scd.Logger,
				// TODO: PrimaryAccessURL
				PrimaryAccessURL: pu,
				AccessURL:        cfg.AccessURL.Value(),
				AppHostname:      scd.AppHostname,
				AppHostnameRegex: scd.AppHostnameRegex,
				RealIPConfig:     scd.RealIPConfig,
				// TODO: AppSecurityKey
				AppSecurityKey:     workspaceapps.SecurityKey{},
				Tracing:            scd.Tracer,
				PrometheusRegistry: scd.PrometheusRegistry,
				APIRateLimit:       int(cfg.RateLimit.API.Value()),
				SecureAuthCookie:   cfg.SecureAuthCookie.Value(),
				// TODO: DisablePathApps
				DisablePathApps: false,
				// TODO: ProxySessionToken
				ProxySessionToken: "",
			})
			if err != nil {
				return xerrors.Errorf("create workspace proxy: %w", err)
			}

			shutdownConnsCtx, shutdownConns := context.WithCancel(ctx)
			defer shutdownConns()
			// ReadHeaderTimeout is purposefully not enabled. It caused some
			// issues with websockets over the dev tunnel.
			// See: https://github.com/coder/coder/pull/3730
			//nolint:gosec
			httpServer := &http.Server{
				// These errors are typically noise like "TLS: EOF". Vault does
				// similar:
				// https://github.com/hashicorp/vault/blob/e2490059d0711635e529a4efcbaa1b26998d6e1c/command/server.go#L2714
				ErrorLog: log.New(io.Discard, "", 0),
				Handler:  proxy.Handler,
				BaseContext: func(_ net.Listener) context.Context {
					return shutdownConnsCtx
				},
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = httpServer.Shutdown(ctx)
			}()

			// TODO: So this obviously is not going to work well.
			errCh := make(chan error, 1)
			go pprof.Do(ctx, pprof.Labels("service", "workspace-proxy"), func(ctx context.Context) {
				errCh <- scd.HTTPServers.Serve(httpServer)
			})

			cliui.Infof(inv.Stdout, "\n==> Logs will stream in below (press ctrl+c to gracefully exit):")

			// Updates the systemd status from activating to activated.
			_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
			if err != nil {
				return xerrors.Errorf("notify systemd: %w", err)
			}

			// Currently there is no way to ask the server to shut
			// itself down, so any exit signal will result in a non-zero
			// exit of the server.
			var exitErr error
			select {
			case exitErr = <-errCh:
			case <-scd.NotifyCtx.Done():
				exitErr = scd.NotifyCtx.Err()
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Bold.Render(
					"Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit",
				))
			}

			if exitErr != nil && !xerrors.Is(exitErr, context.Canceled) {
				cliui.Errorf(inv.Stderr, "Unexpected error, shutting down server: %s\n", exitErr)
			}

			// Begin clean shut down stage, we try to shut down services
			// gracefully in an order that gives the best experience.
			// This procedure should not differ greatly from the order
			// of `defer`s in this function, but allows us to inform
			// the user about what's going on and handle errors more
			// explicitly.

			_, err = daemon.SdNotify(false, daemon.SdNotifyStopping)
			if err != nil {
				cliui.Errorf(inv.Stderr, "Notify systemd failed: %s", err)
			}

			// Stop accepting new connections without interrupting
			// in-flight requests, give in-flight requests 5 seconds to
			// complete.
			cliui.Info(inv.Stdout, "Shutting down API server..."+"\n")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err = httpServer.Shutdown(ctx)
			if err != nil {
				cliui.Errorf(inv.Stderr, "API server shutdown took longer than 3s: %s\n", err)
			} else {
				cliui.Info(inv.Stdout, "Gracefully shut down API server\n")
			}
			// Cancel any remaining in-flight requests.
			shutdownConns()

			// Trigger context cancellation for any remaining services.
			scd.Close()

			switch {
			case xerrors.Is(exitErr, context.DeadlineExceeded):
				cliui.Warnf(inv.Stderr, "Graceful shutdown timed out")
				// Errors here cause a significant number of benign CI failures.
				return nil
			case xerrors.Is(exitErr, context.Canceled):
				return nil
			case exitErr != nil:
				return xerrors.Errorf("graceful shutdown: %w", exitErr)
			default:
				return nil
			}
		},
	}

	return cmd
}
