module github.com/nanzhong/tester

go 1.13

require (
	github.com/go-redis/redis/v7 v7.0.0-beta.4
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.3
	github.com/gorilla/sessions v1.2.0
	github.com/gorilla/websocket v1.4.1 // indirect
	github.com/lestrrat-go/jwx v0.9.0 // indirect
	github.com/markbates/pkger v0.14.0
	github.com/nlopes/slack v0.6.0
	github.com/okta/okta-jwt-verifier-golang v0.1.0
	github.com/prometheus/client_golang v1.1.1-0.20191012124942-3ddc3cfbe565
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/spf13/viper v1.4.0
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200107162124-548cf772de50 // indirect
	golang.org/x/text v0.3.2 // indirect
)

replace github.com/nlopes/slack => github.com/nanzhong/slack v0.6.1-0.20200118044918-a49464de8ae8
