package main

import "github.com/nanzhong/tester"

type config struct {
	Packages []tester.Package `json:"packages"`
	Slack    *slackConfig     `json:"slack"`
}

type slackConfig struct {
	Username      string `json:"username"`
	WebhookURL    string `json:"webhook_url"`
	SigningSecret string `json:"signing_secret"`
}
