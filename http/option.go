package http

import (
	"github.com/nanzhong/tester/alerting"
	"github.com/nanzhong/tester/db"
	"github.com/nanzhong/tester/slack"
)

// Option is used to inject dependencies into a Server on creation.
type Option func(*options)

type options struct {
	db           db.DB
	alertManager *alerting.AlertManager
	slackApp     *slack.App
}

// WithDB allows configuring a DB.
func WithDB(db db.DB) Option {
	return func(opts *options) {
		opts.db = db
	}
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
