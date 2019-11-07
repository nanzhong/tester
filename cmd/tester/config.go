package main

import "github.com/nanzhong/tester/runner"

type config struct {
	RunConfigs []runner.TBRunConfig `json:"run_configs"`
}
