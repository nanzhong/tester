package main

import "github.com/nanzhong/tester"

type config struct {
	Packages  []tester.Package `json:"packages"`
	Scheduler *schedulerConfig `json:"scheduler"`
	Slack     *slackConfig     `json:"slack"`
}

type schedulerConfig struct {
	RunTimeout string `json:"run_timeout"`
}

type slackConfig struct {
	CustomChannels map[string][]string `json:"custom_channels"`
}
