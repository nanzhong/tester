package main

import (
	"context"
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
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var opts []runner.Option
		if viper.GetString("tester-addr") != "" {
			opts = append(opts, runner.WithTesterAddr(viper.GetString("tester-addr")))
		}

		config := runner.TBRunConfig{
			Path:    args[0],
			Timeout: viper.GetDuration("timeout"),
		}

		runner := runner.New(config, opts...)

		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-c
			log.Println("shutting down")

			{
				// Give one minute for running requests to complete
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)

				log.Printf("attempting to stop runner...")
				err := runner.Stop(ctx)
				if err != nil {
					log.Printf("failed to stop runner: %s", err)
				}

				cancel()
				close(c)
			}
		}()

		log.Printf("starting test runner")
		runner.Run()
		log.Printf("ending test runner")
	},
}

func init() {
	runCmd.Flags().String("tester-addr", "", "The address where the tester server is listening on")
	viper.BindPFlag("tester-addr", runCmd.Flags().Lookup("tester-addr"))
	runCmd.Flags().Duration("timeout", 10*time.Minute, "The timeout for running the tests")
	viper.BindPFlag("timeout", runCmd.Flags().Lookup("timeout"))
}
