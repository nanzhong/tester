module github.com/nanzhong/tester

go 1.13

require (
	github.com/Masterminds/squirrel v1.4.0
	github.com/gofrs/uuid v3.3.0+incompatible // indirect
	github.com/google/go-cmp v0.4.0 // indirect
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/sessions v1.2.0
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/jackc/pgconn v1.6.2
	github.com/jackc/pgx/v4 v4.7.2
	github.com/jackc/tern v1.12.1
	github.com/lestrrat-go/jwx v0.9.0 // indirect
	github.com/lib/pq v1.3.0
	github.com/markbates/pkger v0.17.0
	github.com/mitchellh/reflectwalk v1.0.1 // indirect
	github.com/okta/okta-jwt-verifier-golang v0.1.0
	github.com/prometheus/client_golang v1.3.0
	github.com/slack-go/slack v0.6.6
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.4.0
	github.com/stretchr/testify v1.5.1
	golang.org/x/crypto v0.0.0-20200709230013-948cd5f35899 // indirect
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200625212154-ddb9806d33ae // indirect
)

replace github.com/nlopes/slack => github.com/nanzhong/slack v0.6.1-0.20200118044918-a49464de8ae8
