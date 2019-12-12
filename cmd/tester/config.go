package main

import "github.com/nanzhong/tester"

type config struct {
	Packages []tester.Package `json:"packages"`
	Slack    slackConfig      `json:"slack"`
}

type slackConfig struct {
	AlertWebhook string `json:"alert_webhook"`
}
