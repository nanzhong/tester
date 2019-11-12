package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nanzhong/tester/db"
	testerhttp "github.com/nanzhong/tester/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "sere the web UI",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		l, err := net.Listen("tcp", viper.GetString("addr"))
		if err != nil {
			log.Fatalf("failed to listen on %s\n", viper.GetString("addr"))
		}

		db := &db.MemDB{}

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
			<-c
			log.Println("shutting down")

			{
				// Give one minute for running requests to complete
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)

				log.Printf("attempting to shutdown http server...")
				err := httpServer.Shutdown(ctx)
				if err != nil {
					log.Printf("failed to shutdown http server: %s", err)
				}

				cancel()
				close(c)
			}
		}()

		log.Printf("serving on %s", viper.GetString("addr"))
		err = httpServer.Serve(l)
		log.Printf("serving ended: %s", err)
	},
}

func init() {
	serveCmd.Flags().String("addr", "0.0.0.0:8080", "The address to serve on")
	viper.BindPFlag("addr", serveCmd.Flags().Lookup("addr"))
}
