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
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		opts := []runner.Option{runner.WithTesterAddr(viper.GetString("run-tester-addr"))}
		if apiKey := viper.GetString("run-api-key"); apiKey != "" {
			opts = append(opts, runner.WithAPIKey(apiKey))
		}
		if testBinsPath := viper.GetString("run-test-bins-path"); testBinsPath != "" {
			opts = append(opts, runner.WithTestBinPath(testBinsPath))
		}
		if localTestBinsOnly := viper.GetBool("run-local-test-bins-only"); localTestBinsOnly {
			opts = append(opts, runner.WithLocalTestBinsOnly())
		}

		runner, err := runner.New(opts...)
		if err != nil {
			log.Fatalf("failed to construct runner: %s", err)
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
	runCmd.Flags().String("tester-addr", "http://0.0.0.0:8080", "The address where the tester server is listening on")
	viper.BindPFlag("run-tester-addr", runCmd.Flags().Lookup("tester-addr"))

	runCmd.Flags().String("api-key", "", "Symmetric key for API Auth")
	viper.BindPFlag("run-api-key", runCmd.Flags().Lookup("api-key"))

	runCmd.Flags().String("test-bins-path", "", "Path to look for and store test binaries")
	viper.BindPFlag("run-test-bins-path", runCmd.Flags().Lookup("test-bins-path"))

	runCmd.Flags().Bool("local-test-bins-only", false, "Disables downloading remote test binaries")
	viper.BindPFlag("run-local-test-bins-only", runCmd.Flags().Lookup("local-test-bins-only"))
}
