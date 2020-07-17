package http

import (
	"github.com/nanzhong/tester/alerting"
	"github.com/nanzhong/tester/slack"
)

// Option is used to inject dependencies into a Server on creation.
type Option func(*options)

type options struct {
	alertManager *alerting.AlertManager
	slackApp     *slack.App
	apiKey       string
}

// WithAlertManager allows configuring a custom alert manager.
func WithAlertManager(am *alerting.AlertManager) Option {
	return func(opts *options) {
		opts.alertManager = am
	}
}

// WithSlackApp allows configuring a slack app integration.
func WithSlackApp(app *slack.App) Option {
	return func(opts *options) {
		opts.slackApp = app
	}
}

// WithAPIKey allows configuring a symmetric key for api auth.
func WithAPIKey(key string) Option {
	return func(opts *options) {
		opts.apiKey = key
	}
}
