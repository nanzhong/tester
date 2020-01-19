package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v7"
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

		var httpOpts []testerhttp.Option
		var dbStore db.DB
		if viper.GetString("serve-redis-url") != "" {
			log.Printf("configuring redis backend")
			dbStore, err = configureRedis()
			if err != nil {
				log.Fatal("failed to configure redis: %w", err)
			}
		} else {
			log.Printf("configuring memory backend")
			dbStore = &db.MemDB{}
		}
		httpOpts = append(httpOpts, testerhttp.WithDB(dbStore))

		log.Print("configuring scheduler")
		var schedulerOpts []scheduler.Option
		if cfg.Scheduler != nil {
			if cfg.Scheduler.RunTimeout != "" {
				timeout, err := time.ParseDuration(cfg.Scheduler.RunTimeout)
				if err != nil {
					log.Fatal("invalid run timeout: %s", cfg.Scheduler.RunTimeout)
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
		if cfg.Slack != nil {
			log.Print("configuring slack")
			opts := []slack.Option{
				slack.WithScheduler(scheduler),
				slack.WithBaseURL(baseURL),
			}
			if cfg.Slack.Username != "" {
				opts = append(opts, slack.WithUsername(cfg.Slack.Username))
			}
			if cfg.Slack.WebhookURL != "" {
				opts = append(opts, slack.WithWebhookURL(cfg.Slack.WebhookURL))
			}
			if cfg.Slack.SigningSecret != "" {
				opts = append(opts, slack.WithSigningSecret(cfg.Slack.SigningSecret))
			}
			slackApp = slack.NewApp(opts...)
			alertManager.RegisterAlerter(slackApp)
			httpOpts = append(httpOpts, testerhttp.WithSlackApp(slackApp))
		}

		uiHandler := testerhttp.NewUIHandler(httpOpts...)
		apiHandler := testerhttp.NewAPIHandler(httpOpts...)

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
		go func() {
			defer close(done)
			<-done

			log.Println("shutting down")
			{
				// Give one minute for running requests to complete
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
				defer cancel()

				var eg errgroup.Group
				eg.Go(func() error {
					log.Printf("attempting to shutdown http server")
					return httpServer.Shutdown(ctx)
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
			log.Print("starting scheduler")
			scheduler.Run()
			return nil
		})
		eg.Go(func() error {
			ticker := time.NewTicker(15 * time.Second)
			for {
				select {
				case <-done:
					return nil
				case <-ticker.C:
					err := dbStore.Archive(context.Background())
					if err != nil {
						log.Printf("failed to archive results: %w", err)
					}
				}
			}
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

	serveCmd.Flags().String("redis-url", "", "The url string of redis")
	viper.BindPFlag("serve-redis-url", serveCmd.Flags().Lookup("redis-url"))

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

func configureRedis() (db.DB, error) {
	redisURL := viper.GetString("serve-redis-url")

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	redisClient := redis.NewClient(opt)

	_, err = redisClient.Ping().Result()
	if err != nil {
		return nil, fmt.Errorf("verifying redis connectivity: %w", err)
	}
	return db.NewRedis(redisClient), nil
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
