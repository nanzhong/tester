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
	Username       string              `json:"username"`
	WebhookURL     string              `json:"webhook_url"`
	SigningSecret  string              `json:"signing_secret"`
	CustomChannels map[string][]string `json:"custom_channels"`
}
