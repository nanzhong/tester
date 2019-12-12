package main

import "github.com/nanzhong/tester"

type config struct {
	Packages []tester.Package `json:"packages"`
	Alerting *alertingConfig  `json:"alerting"`
}

type alertingConfig struct {
	Slack *slackConfig `json:"slack"`
}

type slackConfig struct {
	Username string `json:"username"`
	Webhook  string `json:"webhook"`
}
