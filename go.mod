module shizukubot

go 1.26

require (
	github.com/go-resty/resty/v2 v2.6.0
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/sessions v1.2.2
	github.com/gorilla/websocket v1.5.1
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/mxschmitt/playwright-go v0.6100.0
	github.com/shirou/gopsutil/v3 v3.24.5
	github.com/tencent-connect/botgo v0.14.0
	golang.org/x/crypto v0.54.0
	golang.org/x/oauth2 v0.23.0
)

replace github.com/tencent-connect/botgo => ./botgo-master

require (
	github.com/deckarep/golang-set/v2 v2.8.0 // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/go-jose/go-jose/v3 v3.0.5 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tidwall/gjson v1.17.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/traefik/yaegi v0.16.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
