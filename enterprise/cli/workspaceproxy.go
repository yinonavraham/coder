package cli

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/autobuild/executor"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
	"github.com/coder/coder/coderd/database/dbpurge"
	"github.com/coder/coder/coderd/devtunnel"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/coderd/gitsshkey"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/coderd/prometheusmetrics"
	"github.com/coder/coder/coderd/schedule"
	"github.com/coder/coder/coderd/telemetry"
	"github.com/coder/coder/coderd/updatecheck"
	"github.com/coder/coder/coderd/util/slice"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/provisionerd"
	"github.com/coder/coder/tailnet"
	"github.com/coder/wgtunnel/tunnelsdk"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-systemd/daemon"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/mod/semver"
	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/cli"

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
			// Main command context for managing cancellation of running
			// services.
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			var _ = ctx
			// TODO
			// go dumpHandler(ctx)
			// Disable rate limits if the `--dangerous-disable-rate-limits` flag
			// was specified.
			loginRateLimit := 60
			filesRateLimit := 12
			if cfg.RateLimit.DisableAll {
				cfg.RateLimit.API = -1
				loginRateLimit = -1
				filesRateLimit = -1
			}

			cli.printLogo(inv)
			logger, logCloser, err := cli.BuildLogger(inv, cfg)
			if err != nil {
				return xerrors.Errorf("make logger: %w", err)
			}
			defer logCloser()

			// This line is helpful in tests.
			logger.Debug(ctx, "started debug logging")
			logger.Sync()

			// Register signals early on so that graceful shutdown can't
			// be interrupted by additional signals. Note that we avoid
			// shadowing cancel() (from above) here because notifyStop()
			// restores default behavior for the signals. This protects
			// the shutdown sequence from abruptly terminating things
			// like: database migrations, provisioner work, workspace
			// cleanup in dev-mode, etc.
			//
			// To get out of a graceful shutdown, the user can send
			// SIGQUIT with ctrl+\ or SIGKILL with `kill -9`.
			notifyCtx, notifyStop := signal.NotifyContext(ctx, InterruptSignals...)
			defer notifyStop()

			// Ensure we have a unique cache directory for this process.
			cacheDir := filepath.Join(cfg.CacheDir.String(), uuid.NewString())
			err = os.MkdirAll(cacheDir, 0o700)
			if err != nil {
				return xerrors.Errorf("create cache directory: %w", err)
			}
			defer os.RemoveAll(cacheDir)

			// Clean up idle connections at the end, e.g.
			// embedded-postgres can leave an idle connection
			// which is caught by goleaks.
			defer http.DefaultClient.CloseIdleConnections()

			tracerProvider, sqlDriver := ConfigureTraceProvider(ctx, logger, inv, cfg)

			config := r.createConfig()

			builtinPostgres := false
			// Only use built-in if PostgreSQL URL isn't specified!
			if !cfg.InMemoryDatabase && cfg.PostgresURL == "" {
				var closeFunc func() error
				cliui.Infof(inv.Stdout, "Using built-in PostgreSQL (%s)", config.PostgresPath())
				pgURL, closeFunc, err := startBuiltinPostgres(ctx, config, logger)
				if err != nil {
					return err
				}

				err = cfg.PostgresURL.Set(pgURL)
				if err != nil {
					return err
				}
				builtinPostgres = true
				defer func() {
					cliui.Infof(inv.Stdout, "Stopping built-in PostgreSQL...")
					// Gracefully shut PostgreSQL down!
					if err := closeFunc(); err != nil {
						cliui.Errorf(inv.Stderr, "Failed to stop built-in PostgreSQL: %v", err)
					} else {
						cliui.Infof(inv.Stdout, "Stopped built-in PostgreSQL")
					}
				}()
			}

			httpServers, err := ConfigureHTTPServers(inv, cfg)
			if err != nil {
				return xerrors.Errorf("configure http(s): %w", err)
			}
			defer httpServers.Close()

			// Prefer HTTP because it's less prone to TLS errors over localhost.
			localURL := httpServers.TLSUrl
			if httpServers.HTTPUrl != nil {
				localURL = httpServers.HTTPUrl
			}

			ctx, httpClient, err := configureHTTPClient(
				ctx,
				cfg.TLS.ClientCertFile.String(),
				cfg.TLS.ClientKeyFile.String(),
				cfg.TLS.ClientCAFile.String(),
			)
			if err != nil {
				return xerrors.Errorf("configure http client: %w", err)
			}

			// If the access URL is empty, we attempt to run a reverse-proxy
			// tunnel to make the initial setup really simple.
			var (
				tunnel     *tunnelsdk.Tunnel
				tunnelDone <-chan struct{} = make(chan struct{}, 1)
			)
			if cfg.AccessURL.String() == "" {
				cliui.Infof(inv.Stderr, "Opening tunnel so workspaces can connect to your deployment. For production scenarios, specify an external access URL")
				tunnel, err = devtunnel.New(ctx, logger.Named("devtunnel"), cfg.WgtunnelHost.String())
				if err != nil {
					return xerrors.Errorf("create tunnel: %w", err)
				}
				defer tunnel.Close()
				tunnelDone = tunnel.Wait()
				cfg.AccessURL = clibase.URL(*tunnel.URL)

				if cfg.WildcardAccessURL.String() == "" {
					// Suffixed wildcard access URL.
					u, err := url.Parse(fmt.Sprintf("*--%s", tunnel.URL.Hostname()))
					if err != nil {
						return xerrors.Errorf("parse wildcard url: %w", err)
					}
					cfg.WildcardAccessURL = clibase.URL(*u)
				}
			}

			_, accessURLPortRaw, _ := net.SplitHostPort(cfg.AccessURL.Host)
			if accessURLPortRaw == "" {
				accessURLPortRaw = "80"
				if cfg.AccessURL.Scheme == "https" {
					accessURLPortRaw = "443"
				}
			}

			accessURLPort, err := strconv.Atoi(accessURLPortRaw)
			if err != nil {
				return xerrors.Errorf("parse access URL port: %w", err)
			}

			// Warn the user if the access URL appears to be a loopback address.
			isLocal, err := isLocalURL(ctx, cfg.AccessURL.Value())
			if isLocal || err != nil {
				reason := "could not be resolved"
				if isLocal {
					reason = "isn't externally reachable"
				}
				cliui.Warnf(
					inv.Stderr,
					"The access URL %s %s, this may cause unexpected problems when creating workspaces. Generate a unique *.try.coder.app URL by not specifying an access URL.\n",
					cliui.Styles.Field.Render(cfg.AccessURL.String()), reason,
				)
			}

			// A newline is added before for visibility in terminal output.
			cliui.Infof(inv.Stdout, "\nView the Web UI: %s", cfg.AccessURL.String())

			// Used for zero-trust instance identity with Google Cloud.
			googleTokenValidator, err := idtoken.NewValidator(ctx, option.WithoutAuthentication())
			if err != nil {
				return err
			}

			sshKeygenAlgorithm, err := gitsshkey.ParseAlgorithm(cfg.SSHKeygenAlgorithm.String())
			if err != nil {
				return xerrors.Errorf("parse ssh keygen algorithm %s: %w", cfg.SSHKeygenAlgorithm, err)
			}

			defaultRegion := &tailcfg.DERPRegion{
				EmbeddedRelay: true,
				RegionID:      int(cfg.DERP.Server.RegionID.Value()),
				RegionCode:    cfg.DERP.Server.RegionCode.String(),
				RegionName:    cfg.DERP.Server.RegionName.String(),
				Nodes: []*tailcfg.DERPNode{{
					Name:      fmt.Sprintf("%db", cfg.DERP.Server.RegionID),
					RegionID:  int(cfg.DERP.Server.RegionID.Value()),
					HostName:  cfg.AccessURL.Value().Hostname(),
					DERPPort:  accessURLPort,
					STUNPort:  -1,
					ForceHTTP: cfg.AccessURL.Scheme == "http",
				}},
			}
			if !cfg.DERP.Server.Enable {
				defaultRegion = nil
			}
			derpMap, err := tailnet.NewDERPMap(
				ctx, defaultRegion, cfg.DERP.Server.STUNAddresses,
				cfg.DERP.Config.URL.String(), cfg.DERP.Config.Path.String(),
			)
			if err != nil {
				return xerrors.Errorf("create derp map: %w", err)
			}

			appHostname := cfg.WildcardAccessURL.String()
			var appHostnameRegex *regexp.Regexp
			if appHostname != "" {
				appHostnameRegex, err = httpapi.CompileHostnamePattern(appHostname)
				if err != nil {
					return xerrors.Errorf("parse wildcard access URL %q: %w", appHostname, err)
				}
			}

			gitAuthEnv, err := ReadGitAuthProvidersFromEnv(os.Environ())
			if err != nil {
				return xerrors.Errorf("read git auth providers from env: %w", err)
			}

			cfg.GitAuthProviders.Value = append(cfg.GitAuthProviders.Value, gitAuthEnv...)
			gitAuthConfigs, err := gitauth.ConvertConfig(
				cfg.GitAuthProviders.Value,
				cfg.AccessURL.Value(),
			)
			if err != nil {
				return xerrors.Errorf("convert git auth config: %w", err)
			}
			for _, c := range gitAuthConfigs {
				logger.Debug(
					ctx, "loaded git auth config",
					slog.F("id", c.ID),
				)
			}

			realIPConfig, err := httpmw.ParseRealIPConfig(cfg.ProxyTrustedHeaders, cfg.ProxyTrustedOrigins)
			if err != nil {
				return xerrors.Errorf("parse real ip config: %w", err)
			}

			configSSHOptions, err := cfg.SSHConfig.ParseOptions()
			if err != nil {
				return xerrors.Errorf("parse ssh config options %q: %w", cfg.SSHConfig.SSHConfigOptions.String(), err)
			}

			options := &coderd.Options{
				AccessURL:                   cfg.AccessURL.Value(),
				AppHostname:                 appHostname,
				AppHostnameRegex:            appHostnameRegex,
				Logger:                      logger.Named("coderd"),
				Database:                    dbfake.New(),
				DERPMap:                     derpMap,
				Pubsub:                      database.NewPubsubInMemory(),
				CacheDir:                    cacheDir,
				GoogleTokenValidator:        googleTokenValidator,
				GitAuthConfigs:              gitAuthConfigs,
				RealIPConfig:                realIPConfig,
				SecureAuthCookie:            cfg.SecureAuthCookie.Value(),
				SSHKeygenAlgorithm:          sshKeygenAlgorithm,
				TracerProvider:              tracerProvider,
				Telemetry:                   telemetry.NewNoop(),
				MetricsCacheRefreshInterval: cfg.MetricsCacheRefreshInterval.Value(),
				AgentStatsRefreshInterval:   cfg.AgentStatRefreshInterval.Value(),
				DeploymentValues:            cfg,
				PrometheusRegistry:          prometheus.NewRegistry(),
				APIRateLimit:                int(cfg.RateLimit.API.Value()),
				LoginRateLimit:              loginRateLimit,
				FilesRateLimit:              filesRateLimit,
				HTTPClient:                  httpClient,
				TemplateScheduleStore:       &atomic.Pointer[schedule.TemplateScheduleStore]{},
				SSHConfig: codersdk.SSHConfigResponse{
					HostnamePrefix:   cfg.SSHConfig.DeploymentName.String(),
					SSHConfigOptions: configSSHOptions,
				},
			}
			if httpServers.TLSConfig != nil {
				options.TLSCertificates = httpServers.TLSConfig.Certificates
			}

			if cfg.StrictTransportSecurity > 0 {
				options.StrictTransportSecurityCfg, err = httpmw.HSTSConfigOptions(
					int(cfg.StrictTransportSecurity.Value()), cfg.StrictTransportSecurityOptions,
				)
				if err != nil {
					return xerrors.Errorf("coderd: setting hsts header failed (options: %v): %w", cfg.StrictTransportSecurityOptions, err)
				}
			}

			if cfg.UpdateCheck {
				options.UpdateCheckOptions = &updatecheck.Options{
					// Avoid spamming GitHub API checking for updates.
					Interval: 24 * time.Hour,
					// Inform server admins of new versions.
					Notify: func(r updatecheck.Result) {
						if semver.Compare(r.Version, buildinfo.Version()) > 0 {
							options.Logger.Info(
								context.Background(),
								"new version of coder available",
								slog.F("new_version", r.Version),
								slog.F("url", r.URL),
								slog.F("upgrade_instructions", "https://coder.com/docs/coder-oss/latest/admin/upgrade"),
							)
						}
					},
				}
			}

			if cfg.OAuth2.Github.ClientSecret != "" {
				options.GithubOAuth2Config, err = configureGithubOAuth2(cfg.AccessURL.Value(),
					cfg.OAuth2.Github.ClientID.String(),
					cfg.OAuth2.Github.ClientSecret.String(),
					cfg.OAuth2.Github.AllowSignups.Value(),
					cfg.OAuth2.Github.AllowEveryone.Value(),
					cfg.OAuth2.Github.AllowedOrgs,
					cfg.OAuth2.Github.AllowedTeams,
					cfg.OAuth2.Github.EnterpriseBaseURL.String(),
				)
				if err != nil {
					return xerrors.Errorf("configure github oauth2: %w", err)
				}
			}

			if cfg.OIDC.ClientSecret != "" {
				if cfg.OIDC.ClientID == "" {
					return xerrors.Errorf("OIDC client ID be set!")
				}
				if cfg.OIDC.IssuerURL == "" {
					return xerrors.Errorf("OIDC issuer URL must be set!")
				}

				if cfg.OIDC.IgnoreEmailVerified {
					logger.Warn(ctx, "coder will not check email_verified for OIDC logins")
				}

				oidcProvider, err := oidc.NewProvider(
					ctx, cfg.OIDC.IssuerURL.String(),
				)
				if err != nil {
					return xerrors.Errorf("configure oidc provider: %w", err)
				}
				redirectURL, err := cfg.AccessURL.Value().Parse("/api/v2/users/oidc/callback")
				if err != nil {
					return xerrors.Errorf("parse oidc oauth callback url: %w", err)
				}
				// If the scopes contain 'groups', we enable group support.
				// Do not override any custom value set by the user.
				if slice.Contains(cfg.OIDC.Scopes, "groups") && cfg.OIDC.GroupField == "" {
					cfg.OIDC.GroupField = "groups"
				}
				options.OIDCConfig = &coderd.OIDCConfig{
					OAuth2Config: &oauth2.Config{
						ClientID:     cfg.OIDC.ClientID.String(),
						ClientSecret: cfg.OIDC.ClientSecret.String(),
						RedirectURL:  redirectURL.String(),
						Endpoint:     oidcProvider.Endpoint(),
						Scopes:       cfg.OIDC.Scopes,
					},
					Provider: oidcProvider,
					Verifier: oidcProvider.Verifier(&oidc.Config{
						ClientID: cfg.OIDC.ClientID.String(),
					}),
					EmailDomain:         cfg.OIDC.EmailDomain,
					AllowSignups:        cfg.OIDC.AllowSignups.Value(),
					UsernameField:       cfg.OIDC.UsernameField.String(),
					EmailField:          cfg.OIDC.EmailField.String(),
					AuthURLParams:       cfg.OIDC.AuthURLParams.Value,
					IgnoreUserInfo:      cfg.OIDC.IgnoreUserInfo.Value(),
					GroupField:          cfg.OIDC.GroupField.String(),
					GroupMapping:        cfg.OIDC.GroupMapping.Value,
					SignInText:          cfg.OIDC.SignInText.String(),
					IconURL:             cfg.OIDC.IconURL.String(),
					IgnoreEmailVerified: cfg.OIDC.IgnoreEmailVerified.Value(),
				}
			}

			if cfg.InMemoryDatabase {
				options.Database = dbfake.New()
				options.Pubsub = database.NewPubsubInMemory()
			} else {
				sqlDB, err := connectToPostgres(ctx, logger, sqlDriver, cfg.PostgresURL.String())
				if err != nil {
					return xerrors.Errorf("connect to postgres: %w", err)
				}
				defer func() {
					_ = sqlDB.Close()
				}()

				options.Database = database.New(sqlDB)
				options.Pubsub, err = database.NewPubsub(ctx, sqlDB, cfg.PostgresURL.String())
				if err != nil {
					return xerrors.Errorf("create pubsub: %w", err)
				}
				defer options.Pubsub.Close()
			}

			var deploymentID string
			err = options.Database.InTx(func(tx database.Store) error {
				// This will block until the lock is acquired, and will be
				// automatically released when the transaction ends.
				err := tx.AcquireLock(ctx, database.LockIDDeploymentSetup)
				if err != nil {
					return xerrors.Errorf("acquire lock: %w", err)
				}

				deploymentID, err = tx.GetDeploymentID(ctx)
				if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
					return xerrors.Errorf("get deployment id: %w", err)
				}
				if deploymentID == "" {
					deploymentID = uuid.NewString()
					err = tx.InsertDeploymentID(ctx, deploymentID)
					if err != nil {
						return xerrors.Errorf("set deployment id: %w", err)
					}
				}

				// Read the app signing key from the DB. We store it hex encoded
				// since the config table uses strings for the value and we
				// don't want to deal with automatic encoding issues.
				appSecurityKeyStr, err := tx.GetAppSecurityKey(ctx)
				if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
					return xerrors.Errorf("get app signing key: %w", err)
				}
				// If the string in the DB is an invalid hex string or the
				// length is not equal to the current key length, generate a new
				// one.
				//
				// If the key is regenerated, old signed tokens and encrypted
				// strings will become invalid. New signed app tokens will be
				// generated automatically on failure. Any workspace app token
				// smuggling operations in progress may fail, although with a
				// helpful error.
				if decoded, err := hex.DecodeString(appSecurityKeyStr); err != nil || len(decoded) != len(workspaceapps.SecurityKey{}) {
					b := make([]byte, len(workspaceapps.SecurityKey{}))
					_, err := rand.Read(b)
					if err != nil {
						return xerrors.Errorf("generate fresh app signing key: %w", err)
					}

					appSecurityKeyStr = hex.EncodeToString(b)
					err = tx.UpsertAppSecurityKey(ctx, appSecurityKeyStr)
					if err != nil {
						return xerrors.Errorf("insert freshly generated app signing key to database: %w", err)
					}
				}

				appSecurityKey, err := workspaceapps.KeyFromString(appSecurityKeyStr)
				if err != nil {
					return xerrors.Errorf("decode app signing key from database: %w", err)
				}

				options.AppSecurityKey = appSecurityKey
				return nil
			}, nil)
			if err != nil {
				return err
			}

			if cfg.Telemetry.Enable {
				gitAuth := make([]telemetry.GitAuth, 0)
				// TODO:
				var gitAuthConfigs []codersdk.GitAuthConfig
				for _, cfg := range gitAuthConfigs {
					gitAuth = append(gitAuth, telemetry.GitAuth{
						Type: cfg.Type,
					})
				}

				options.Telemetry, err = telemetry.New(telemetry.Options{
					BuiltinPostgres:    builtinPostgres,
					DeploymentID:       deploymentID,
					Database:           options.Database,
					Logger:             logger.Named("telemetry"),
					URL:                cfg.Telemetry.URL.Value(),
					Wildcard:           cfg.WildcardAccessURL.String() != "",
					DERPServerRelayURL: cfg.DERP.Server.RelayURL.String(),
					GitAuth:            gitAuth,
					GitHubOAuth:        cfg.OAuth2.Github.ClientID != "",
					OIDCAuth:           cfg.OIDC.ClientID != "",
					OIDCIssuerURL:      cfg.OIDC.IssuerURL.String(),
					Prometheus:         cfg.Prometheus.Enable.Value(),
					STUN:               len(cfg.DERP.Server.STUNAddresses) != 0,
					Tunnel:             tunnel != nil,
				})
				if err != nil {
					return xerrors.Errorf("create telemetry reporter: %w", err)
				}
				defer options.Telemetry.Close()
			}

			// This prevents the pprof import from being accidentally deleted.
			_ = pprof.Handler
			if cfg.Pprof.Enable {
				//nolint:revive
				defer serveHandler(ctx, logger, nil, cfg.Pprof.Address.String(), "pprof")()
			}
			if cfg.Prometheus.Enable {
				options.PrometheusRegistry.MustRegister(collectors.NewGoCollector())
				options.PrometheusRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

				closeUsersFunc, err := prometheusmetrics.ActiveUsers(ctx, options.PrometheusRegistry, options.Database, 0)
				if err != nil {
					return xerrors.Errorf("register active users prometheus metric: %w", err)
				}
				defer closeUsersFunc()

				closeWorkspacesFunc, err := prometheusmetrics.Workspaces(ctx, options.PrometheusRegistry, options.Database, 0)
				if err != nil {
					return xerrors.Errorf("register workspaces prometheus metric: %w", err)
				}
				defer closeWorkspacesFunc()

				//nolint:revive
				defer serveHandler(ctx, logger, promhttp.InstrumentMetricHandler(
					options.PrometheusRegistry, promhttp.HandlerFor(options.PrometheusRegistry, promhttp.HandlerOpts{}),
				), cfg.Prometheus.Address.String(), "prometheus")()
			}

			if cfg.Swagger.Enable {
				options.SwaggerEndpoint = cfg.Swagger.Enable.Value()
			}

			// We use a separate coderAPICloser so the Enterprise API
			// can have it's own close functions. This is cleaner
			// than abstracting the Coder API itself.
			coderAPI, coderAPICloser, err := newAPI(ctx, options)
			if err != nil {
				return xerrors.Errorf("create coder API: %w", err)
			}

			if cfg.Prometheus.Enable {
				// Agent metrics require reference to the tailnet coordinator, so must be initiated after Coder API.
				closeAgentsFunc, err := prometheusmetrics.Agents(ctx, logger, options.PrometheusRegistry, coderAPI.Database, &coderAPI.TailnetCoordinator, options.DERPMap, coderAPI.Options.AgentInactiveDisconnectTimeout, 0)
				if err != nil {
					return xerrors.Errorf("register agents prometheus metric: %w", err)
				}
				defer closeAgentsFunc()
			}

			client := codersdk.New(localURL)
			if localURL.Scheme == "https" && isLocalhost(localURL.Hostname()) {
				// The certificate will likely be self-signed or for a different
				// hostname, so we need to skip verification.
				client.HTTPClient.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{
						//nolint:gosec
						InsecureSkipVerify: true,
					},
				}
			}
			defer client.HTTPClient.CloseIdleConnections()

			// This is helpful for tests, but can be silently ignored.
			// Coder may be ran as users that don't have permission to write in the homedir,
			// such as via the systemd service.
			err = config.URL().Write(client.URL.String())
			if err != nil && flag.Lookup("test.v") != nil {
				return xerrors.Errorf("write config url: %w", err)
			}

			// Since errCh only has one buffered slot, all routines
			// sending on it must be wrapped in a select/default to
			// avoid leaving dangling goroutines waiting for the
			// channel to be consumed.
			errCh := make(chan error, 1)
			provisionerDaemons := make([]*provisionerd.Server, 0)
			defer func() {
				// We have no graceful shutdown of provisionerDaemons
				// here because that's handled at the end of main, this
				// is here in case the program exits early.
				for _, daemon := range provisionerDaemons {
					_ = daemon.Close()
				}
			}()

			var provisionerdWaitGroup sync.WaitGroup
			defer provisionerdWaitGroup.Wait()
			provisionerdMetrics := provisionerd.NewMetrics(options.PrometheusRegistry)
			for i := int64(0); i < cfg.Provisioner.Daemons.Value(); i++ {
				daemonCacheDir := filepath.Join(cacheDir, fmt.Sprintf("provisioner-%d", i))
				daemon, err := newProvisionerDaemon(
					ctx, coderAPI, provisionerdMetrics, logger, cfg, daemonCacheDir, errCh, false, &provisionerdWaitGroup,
				)
				if err != nil {
					return xerrors.Errorf("create provisioner daemon: %w", err)
				}
				provisionerDaemons = append(provisionerDaemons, daemon)
			}

			shutdownConnsCtx, shutdownConns := context.WithCancel(ctx)
			defer shutdownConns()

			// Ensures that old database entries are cleaned up over time!
			purger := dbpurge.New(ctx, logger, options.Database)
			defer purger.Close()

			// Wrap the server in middleware that redirects to the access URL if
			// the request is not to a local IP.
			var handler http.Handler = coderAPI.RootHandler
			if cfg.RedirectToAccessURL {
				handler = redirectToAccessURL(handler, cfg.AccessURL.Value(), tunnel != nil, appHostnameRegex)
			}

			// ReadHeaderTimeout is purposefully not enabled. It caused some
			// issues with websockets over the dev tunnel.
			// See: https://github.com/coder/coder/pull/3730
			//nolint:gosec
			httpServer := &http.Server{
				// These errors are typically noise like "TLS: EOF". Vault does
				// similar:
				// https://github.com/hashicorp/vault/blob/e2490059d0711635e529a4efcbaa1b26998d6e1c/command/server.go#L2714
				ErrorLog: log.New(io.Discard, "", 0),
				Handler:  handler,
				BaseContext: func(_ net.Listener) context.Context {
					return shutdownConnsCtx
				},
			}
			defer func() {
				_ = shutdownWithTimeout(httpServer.Shutdown, 5*time.Second)
			}()

			// We call this in the routine so we can kill the other listeners if
			// one of them fails.
			closeListenersNow := func() {
				httpServers.Close()
				if tunnel != nil {
					_ = tunnel.Listener.Close()
				}
			}

			eg := errgroup.Group{}
			eg.Go(func() error {
				defer closeListenersNow()
				return httpServers.Serve(httpServer)
			})
			if tunnel != nil {
				eg.Go(func() error {
					defer closeListenersNow()
					return httpServer.Serve(tunnel.Listener)
				})
			}

			go func() {
				select {
				case errCh <- eg.Wait():
				default:
				}
			}()

			cliui.Infof(inv.Stdout, "\n==> Logs will stream in below (press ctrl+c to gracefully exit):")

			// Updates the systemd status from activating to activated.
			_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
			if err != nil {
				return xerrors.Errorf("notify systemd: %w", err)
			}

			autobuildPoller := time.NewTicker(cfg.AutobuildPollInterval.Value())
			defer autobuildPoller.Stop()
			autobuildExecutor := executor.New(ctx, options.Database, coderAPI.TemplateScheduleStore, logger, autobuildPoller.C)
			autobuildExecutor.Run()

			// Currently there is no way to ask the server to shut
			// itself down, so any exit signal will result in a non-zero
			// exit of the server.
			var exitErr error
			select {
			case <-notifyCtx.Done():
				exitErr = notifyCtx.Err()
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Bold.Render(
					"Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit",
				))
			case <-tunnelDone:
				exitErr = xerrors.New("dev tunnel closed unexpectedly")
			case exitErr = <-errCh:
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
			err = shutdownWithTimeout(httpServer.Shutdown, 3*time.Second)
			if err != nil {
				cliui.Errorf(inv.Stderr, "API server shutdown took longer than 3s: %s\n", err)
			} else {
				cliui.Info(inv.Stdout, "Gracefully shut down API server\n")
			}
			// Cancel any remaining in-flight requests.
			shutdownConns()

			// Shut down provisioners before waiting for WebSockets
			// connections to close.
			var wg sync.WaitGroup
			for i, provisionerDaemon := range provisionerDaemons {
				id := i + 1
				provisionerDaemon := provisionerDaemon
				wg.Add(1)
				go func() {
					defer wg.Done()

					if ok, _ := inv.ParsedFlags().GetBool(varVerbose); ok {
						cliui.Infof(inv.Stdout, "Shutting down provisioner daemon %d...\n", id)
					}
					err := shutdownWithTimeout(provisionerDaemon.Shutdown, 5*time.Second)
					if err != nil {
						cliui.Errorf(inv.Stderr, "Failed to shutdown provisioner daemon %d: %s\n", id, err)
						return
					}
					err = provisionerDaemon.Close()
					if err != nil {
						cliui.Errorf(inv.Stderr, "Close provisioner daemon %d: %s\n", id, err)
						return
					}
					if ok, _ := inv.ParsedFlags().GetBool(varVerbose); ok {
						cliui.Infof(inv.Stdout, "Gracefully shut down provisioner daemon %d\n", id)
					}
				}()
			}
			wg.Wait()

			cliui.Info(inv.Stdout, "Waiting for WebSocket connections to close..."+"\n")
			_ = coderAPICloser.Close()
			cliui.Info(inv.Stdout, "Done waiting for WebSocket connections"+"\n")

			// Close tunnel after we no longer have in-flight connections.
			if tunnel != nil {
				cliui.Infof(inv.Stdout, "Waiting for tunnel to close...")
				_ = tunnel.Close()
				<-tunnel.Wait()
				cliui.Infof(inv.Stdout, "Done waiting for tunnel")
			}

			// Ensures a last report can be sent before exit!
			options.Telemetry.Close()

			// Trigger context cancellation for any remaining services.
			cancel()

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
