package http

import "github.com/nanzhong/tester/db"

import "github.com/nanzhong/tester/alerting"

// Option is used to inject dependencies into a Server on creation.
type Option func(*options)

type options struct {
	db           db.DB
	alertManager *alerting.AlertManager
}

// WithDB allows configuring a DB.
func WithDB(db db.DB) Option {
	return func(opts *options) {
		opts.db = db
	}
}

// WithAlertManager allows configuring a custom alert manager
func WithAlertManager(am *alerting.AlertManager) Option {
	return func(opts *options) {
		opts.alertManager = am
	}
}
