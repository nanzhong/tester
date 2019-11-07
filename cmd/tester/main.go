package main

import (
	"os"
	"strings"

	"github.com/spf13/viper"
)

func init() {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func main() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
