package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/nanzhong/tester/alerting"
	"github.com/nanzhong/tester/db"
	testerhttp "github.com/nanzhong/tester/http"
	"github.com/nanzhong/tester/http/okta"
	"github.com/nanzhong/tester/scheduler"
	"github.com/nanzhong/tester/slack"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "sere the web UI",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		configPath := viper.GetString("serve-config")
		file, err := os.Open(configPath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("config (%s) does not exist", configPath)
			}
			log.Fatalf("failed to read config (%s): %s", configPath, err)
		}
		var cfg config
		err = json.NewDecoder(file).Decode(&cfg)
		if err != nil {
			log.Fatalf("failed to parse config (%s): %s", configPath, err)
		}

		l, err := net.Listen("tcp", viper.GetString("serve-addr"))
		if err != nil {
			log.Fatalf("failed to listen on %s", viper.GetString("serve-addr"))
		}

		pool, err := pgxpool.Connect(context.Background(), viper.GetString("serve-pg-dsn"))
		if err != nil {
			log.Fatalf("failed to connect to db at %s: %s", viper.GetString("serve-addr"), err)
		}
		defer pool.Close()

		dbStore := db.NewPG(pool)
		err = dbStore.Init(context.Background())
		if err != nil {
			log.Fatalf("failed to init db: %s", err)
		}

		var httpOpts []testerhttp.Option
		if apiKey := viper.GetString("serve-api-key"); apiKey != "" {
			httpOpts = append(httpOpts, testerhttp.WithAPIKey(apiKey))
		}

		log.Print("configuring scheduler")
		var schedulerOpts []scheduler.Option
		if cfg.Scheduler != nil {
			if cfg.Scheduler.RunTimeout != "" {
				timeout, err := time.ParseDuration(cfg.Scheduler.RunTimeout)
				if err != nil {
					log.Fatalf("invalid run timeout: %s", cfg.Scheduler.RunTimeout)
				}
				schedulerOpts = append(schedulerOpts, scheduler.WithRunTimeout(timeout))
			}
		}
		scheduler := scheduler.NewScheduler(dbStore, cfg.Packages)

		log.Print("configuring alert manager")
		var (
			alerters []alerting.Alerter
			baseURL  = viper.GetString("serve-base-url")
		)
		alertManager := alerting.NewAlertManager(baseURL, alerters)
		httpOpts = append(httpOpts, testerhttp.WithAlertManager(alertManager))

		var slackApp *slack.App
		if viper.GetString("serve-slack-access-token") != "" &&
			viper.GetString("serve-slack-signing-secret") != "" {
			log.Print("configuring slack")
			opts := []slack.Option{
				slack.WithScheduler(scheduler),
				slack.WithBaseURL(baseURL),
				slack.WithAccessToken(viper.GetString("serve-slack-access-token")),
				slack.WithSigningSecret(viper.GetString("serve-slack-signing-secret")),
				slack.WithDefaultChannels(cfg.Slack.DefaultChannels),
			}
			if cfg.Slack.CustomChannels != nil {
				opts = append(opts, slack.WithCustomChannels(cfg.Slack.CustomChannels))
			}
			slackApp = slack.NewApp(cfg.Packages, opts...)
			alertManager.RegisterAlerter(slackApp)
			httpOpts = append(httpOpts, testerhttp.WithSlackApp(slackApp))
		}

		uiHandler := testerhttp.NewUIHandler(dbStore, cfg.Packages)
		apiHandler := testerhttp.NewAPIHandler(dbStore, httpOpts...)

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.Handle("/api/", apiHandler)

		oktaAuthHandler := configureOktaAuth(uiHandler.RenderError)
		if oktaAuthHandler != nil {
			log.Println("configuring okta auth")
			mux.HandleFunc("/oauth/callback", oktaAuthHandler.AuthCodeCallbackHandler)
			mux.HandleFunc("/", oktaAuthHandler.Ensure(uiHandler.ServeHTTP))
		} else {
			mux.Handle("/", uiHandler)
		}

		httpServer := http.Server{
			Handler: mux,
		}

		done := make(chan os.Signal, 1)
		signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			defer close(done)
			<-done

			log.Println("shutting down")
			{
				cancel()

				// Give one minute for running requests to complete
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
				defer cancel()

				var eg errgroup.Group
				eg.Go(func() error {
					log.Printf("attempting to shutdown http server")
					return httpServer.Shutdown(shutdownCtx)
				})
				eg.Go(func() error {
					log.Printf("attempting to shutdown scheduler")
					scheduler.Stop()
					return nil
				})
				err := eg.Wait()
				if err != nil {
					log.Printf("failed to gracefully shutdown: %s", err)
				}
			}
		}()

		var eg errgroup.Group
		eg.Go(func() error {
			log.Printf("serving on %s", viper.GetString("serve-addr"))
			return httpServer.Serve(l)
		})
		eg.Go(func() error {
			for {
				if _, _, _, _, err := uiHandler.LoadSummaries(ctx); err != nil {
					log.Printf("failed to refresh summaries %s", err)
				}

				select {
				case <-time.After(time.Minute):
				case <-ctx.Done():
					return nil
				}
			}
		})
		eg.Go(func() error {
			log.Print("starting scheduler")
			scheduler.Run()
			return nil
		})
		err = eg.Wait()
		log.Printf("server ended: %s", err)
	},
}

func init() {
	serveCmd.Flags().String("config", "", "Path to the configuration file")
	viper.BindPFlag("serve-config", serveCmd.Flags().Lookup("config"))

	serveCmd.Flags().String("addr", "0.0.0.0:8080", "The address to serve on")
	viper.BindPFlag("serve-addr", serveCmd.Flags().Lookup("addr"))

	serveCmd.Flags().String("base-url", "http://0.0.0.0:8080", "The base url to use for constructing link urls")
	viper.BindPFlag("serve-base-url", serveCmd.Flags().Lookup("base-url"))

	serveCmd.Flags().String("pg-dsn", "", "The postgresql dsn to use.")
	viper.BindPFlag("serve-pg-dsn", serveCmd.Flags().Lookup("pg-dsn"))

	serveCmd.Flags().String("api-key", "", "Symmetric key for API Auth")
	viper.BindPFlag("serve-api-key", serveCmd.Flags().Lookup("api-key"))

	serveCmd.Flags().String("slack-access-token", "", "Slack app access token")
	viper.BindPFlag("serve-slack-access-token", serveCmd.Flags().Lookup("slack-access-token"))
	serveCmd.Flags().String("slack-signing-secret", "", "Slack signing secret")
	viper.BindPFlag("serve-slack-signing-secret", serveCmd.Flags().Lookup("slack-signing-secret"))

	serveCmd.Flags().String("okta-session-key", "", "Okta session key")
	viper.BindPFlag("serve-okta-session-key", serveCmd.Flags().Lookup("okta-session-key"))
	serveCmd.Flags().String("okta-client-id", "", "Okta client ID")
	viper.BindPFlag("serve-okta-client-id", serveCmd.Flags().Lookup("okta-client-id"))
	serveCmd.Flags().String("okta-client-secret", "", "Okta client secret")
	viper.BindPFlag("serve-okta-client-secret", serveCmd.Flags().Lookup("okta-client-secret"))
	serveCmd.Flags().String("okta-issuer", "", "Okta issuer")
	viper.BindPFlag("serve-okta-issuer", serveCmd.Flags().Lookup("okta-issuer"))
	serveCmd.Flags().String("okta-redirect-uri", "", "Okta redirect URI")
	viper.BindPFlag("serve-okta-redirect-uri", serveCmd.Flags().Lookup("okta-redirect-uri"))
}

func configureOktaAuth(errorWriter func(w http.ResponseWriter, r *http.Request, err error, status int)) *okta.AuthHandler {
	sessionKey := viper.GetString("serve-okta-session-key")
	clientID := viper.GetString("serve-okta-client-id")
	clientSecret := viper.GetString("serve-okta-client-secret")
	issuer := viper.GetString("serve-okta-issuer")
	redirectURI := viper.GetString("serve-okta-redirect-uri")

	if sessionKey != "" &&
		clientID != "" &&
		clientSecret != "" &&
		issuer != "" &&
		redirectURI != "" {
		return okta.NewAuthHandler([]byte(sessionKey), clientID, clientSecret, issuer, redirectURI, errorWriter)
	}
	return nil
}
