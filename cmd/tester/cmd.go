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
	"github.com/nanzhong/tester/runner"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cmd = &cobra.Command{
		Use:   "tester",
		Short: "tester is a go test runner",
		Long:  "tester is a go test runner that also presents a web UI for inspecting results and prometheus mettrics for test run statistics",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			var cfg config
			f, err := os.Open(viper.GetString("config"))
			if err != nil {
				log.Fatalf("failed to read config: %s", err)
			}
			err = json.NewDecoder(f).Decode(&cfg)
			if err != nil {
				log.Fatalf("failed to parse config: %s", err)
			}

			var runConfigs []runner.TBRunConfig
			for _, c := range cfg.RunConfigs {
				runConfigs = append(runConfigs, runner.TBRunConfig{
					Path:    c.Path,
					Timeout: time.Duration(c.Timeout),
				})
			}

			l, err := net.Listen("tcp", viper.GetString("addr"))
			if err != nil {
				log.Fatalf("failed to listen on %s\n", viper.GetString("addr"))
			}

			db := &db.MemDB{}

			runner := runner.New(runConfigs, runner.WithDB(db))
			go runner.Run()

			server := testerhttp.New(testerhttp.WithDB(db))

			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			mux.Handle("/", server)

			httpServer := http.Server{
				Handler: mux,
			}

			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-c
				log.Println("shutting down")

				{
					// Give one minute for running requests to complete
					ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)

					log.Printf("attempting to stop runner...")
					err = runner.Stop(ctx)
					if err != nil {
						log.Printf("failed to stop runner: %s\n", err)
					}

					log.Printf("attempting to shutdown http server...")
					err := httpServer.Shutdown(ctx)
					if err != nil {
						log.Printf("failed to shutdown http server: %s\n", err)
					}

					cancel()
					close(c)
				}
			}()

			log.Printf("serving on %s\n", viper.GetString("addr"))
			err = httpServer.Serve(l)
			log.Printf("serving ended: %s\n", err)
		},
	}
)

func init() {
	cmd.Flags().String("addr", "0.0.0.0:8080", "The address to serve on")
	viper.BindPFlag("addr", cmd.Flags().Lookup("addr"))
	cmd.Flags().String("config", "", "The path to the run config file")
	cmd.MarkFlagRequired("config")
	viper.BindPFlag("config", cmd.Flags().Lookup("config"))
}
