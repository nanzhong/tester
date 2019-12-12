package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nanzhong/tester/runner"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "start a test runner",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		file, err := os.Open(viper.GetString("run-packages-config"))
		if err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("packages-config (%s) does not exist", viper.GetString("run-packages-config"))
			}
			log.Fatalf("failed to read packages-config (%s): %s", viper.GetString("run-packages-config"), err)
		}
		var cfg config
		err = json.NewDecoder(file).Decode(&cfg)
		if err != nil {
			log.Fatalf("failed to read package-config (%s): %s", viper.GetString("run-packages-config"), err)
		}

		runner := runner.New(cfg.Packages, runner.WithTesterAddr(viper.GetString("run-tester-addr")))

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

				log.Printf("attempting to stop runner...")
				runner.Stop(ctx)
			}
		}()

		log.Printf("starting test runner")
		runner.Run()
		log.Printf("ending test runner")
	},
}

func init() {
	runCmd.Flags().String("packages-config", "", "Path to the packages configuration file")
	viper.BindPFlag("run-packages-config", runCmd.Flags().Lookup("packages-config"))

	runCmd.Flags().String("tester-addr", "http://0.0.0.0:8080", "The address where the tester server is listening on")
	viper.BindPFlag("run-tester-addr", runCmd.Flags().Lookup("tester-addr"))
}
