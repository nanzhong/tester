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

		db := &db.MemDB{}
		scheduler := scheduler.NewScheduler(cfg.Packages, scheduler.WithDB(db))
		uiHandler := testerhttp.NewUIHandler(testerhttp.WithDB(db))
		apiHandler := testerhttp.NewAPIHandler(testerhttp.WithDB(db))

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.Handle("/api/", apiHandler)
		mux.Handle("/", uiHandler)

		httpServer := http.Server{
			Handler: mux,
		}

		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			defer close(c)
			<-c

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
}
