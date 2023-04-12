package cli

import (
	"io"
	"log"
	"net/http"

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
		Use:   "server",
		Short: "Start a workspace proxy server",
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

			proxy, err := wsproxy.New(&wsproxy.Options{
				Logger:             scd.Logger,
				PrimaryAccessURL:   nil,
				AccessURL:          nil,
				AppHostname:        scd.AppHostname,
				AppHostnameRegex:   scd.AppHostnameRegex,
				RealIPConfig:       scd.RealIPConfig,
				AppSecurityKey:     workspaceapps.SecurityKey{},
				Tracing:            scd.Tracer,
				PrometheusRegistry: nil,
				APIRateLimit:       0,
				SecureAuthCookie:   false,
				DisablePathApps:    false,
				ProxySessionToken:  "",
			})

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
				//BaseContext: func(_ net.Listener) context.Context {
				//	return shutdownConnsCtx
				//},
			}
			//defer func() {
			//	_ = shutdownWithTimeout(httpServer.Shutdown, 5*time.Second)
			//}()

			// TODO: So this obviously is not going to work well.
			return scd.HTTPServers.Serve(httpServer)
		},
	}

	return cmd
}
