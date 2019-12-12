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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/nanzhong/tester/db"
	testerhttp "github.com/nanzhong/tester/http"
	"github.com/nanzhong/tester/scheduler"
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
		file, err := os.Open(viper.GetString("serve-packages-config"))
		if err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("packages-config (%s) does not exist", viper.GetString("serve-packages-config"))
			}
			log.Fatalf("failed to read packages-config (%s): %s", viper.GetString("serve-packages-config"), err)
		}
		var cfg config
		err = json.NewDecoder(file).Decode(&cfg)
		if err != nil {
			log.Fatalf("failed to read package-config (%s): %s", viper.GetString("serve-packages-config"), err)
		}

		l, err := net.Listen("tcp", viper.GetString("serve-addr"))
		if err != nil {
			log.Fatalf("failed to listen on %s", viper.GetString("serve-addr"))
		}

		var dbStore db.DB

		s3Key := viper.GetString("serve-s3-key")
		s3Secret := viper.GetString("serve-s3-secret")
		s3Endpoint := viper.GetString("serve-s3-endpoint")
		s3Region := viper.GetString("serve-s3-region")
		s3Bucket := viper.GetString("serve-s3-bucket")
		if s3Key != "" &&
			s3Secret != "" &&
			s3Endpoint != "" &&
			s3Region != "" &&
			s3Bucket != "" {
			log.Println("configuring s3 for persistence")
			s3Config := &aws.Config{
				Credentials: credentials.NewStaticCredentials(s3Key, s3Secret, ""),
				Endpoint:    &s3Endpoint,
				Region:      &s3Region,
			}
			dbStore = db.NewS3(s3Config, s3Bucket)
		} else {
			dbStore = &db.MemDB{}
		}

		scheduler := scheduler.NewScheduler(cfg.Packages, scheduler.WithDB(dbStore))
		uiHandler := testerhttp.NewUIHandler(testerhttp.WithDB(dbStore))
		apiHandler := testerhttp.NewAPIHandler(testerhttp.WithDB(dbStore))

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.Handle("/api/", apiHandler)
		mux.Handle("/", uiHandler)

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
	serveCmd.Flags().String("packages-config", "", "Path to the packages configuration file")
	serveCmd.MarkFlagRequired("packages-config")
	viper.BindPFlag("serve-packages-config", serveCmd.Flags().Lookup("packages-config"))

	serveCmd.Flags().String("addr", "0.0.0.0:8080", "The address to serve on")
	viper.BindPFlag("serve-addr", serveCmd.Flags().Lookup("addr"))

	serveCmd.Flags().String("s3-bucket", "", "The name of the s3 compatible bucket")
	viper.BindPFlag("serve-s3-bucket", serveCmd.Flags().Lookup("s3-bucket"))
	serveCmd.Flags().String("s3-endpoint", "", "The endpoint for a s3 compatible backend")
	viper.BindPFlag("serve-s3-endpoint", serveCmd.Flags().Lookup("s3-endpoint"))
	serveCmd.Flags().String("s3-region", "", "The region for a s3 compatible backend")
	viper.BindPFlag("serve-s3-region", serveCmd.Flags().Lookup("s3-region"))
	serveCmd.Flags().String("s3-key", "", "The s3 access key id")
	viper.BindPFlag("serve-s3-key", serveCmd.Flags().Lookup("s3-key"))
	serveCmd.Flags().String("s3-secret", "", "The s3 secret access key")
	viper.BindPFlag("serve-s3-secret", serveCmd.Flags().Lookup("s3-secret"))
}
