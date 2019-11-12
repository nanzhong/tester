package http

import "github.com/nanzhong/tester/db"

// Option is used to inject dependencies into a Server on creation.
type Option func(*options)

// WithDB allows configuring a DB.
func WithDB(db *db.MemDB) Option {
	return func(opts *options) {
		opts.db = db
	}
}
