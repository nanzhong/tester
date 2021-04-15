package main

import "github.com/nanzhong/tester"

type config struct {
	Packages  []*tester.Package `json:"packages"`
	Scheduler *schedulerConfig  `json:"scheduler"`
	Slack     *slackConfig      `json:"slack"`
}

type schedulerConfig struct {
	RunTimeout tester.DurationString `json:"run_timeout"`
	RunDelay   tester.DurationString `json:"run_delay"`
}

type slackConfig struct {
	DefaultChannels []string            `json:"default_channels"`
	CustomChannels  map[string][]string `json:"custom_channels"`
}
