// qq-diagnose checks local configuration by default. It never sends messages.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"shizukubot/config"
	"shizukubot/core"
)

func run(args []string, out io.Writer) int {
	flags := flag.NewFlagSet("qq-diagnose", flag.ContinueOnError)
	flags.SetOutput(out)
	file := flags.String("config", "config/config.json", "local credential file (never pass credentials as flags)")
	id := flags.String("bot", "", "configured, user-owned test Bot ID")
	live := flags.Bool("live-auth", false, "explicitly authenticate the selected test Bot with QQ; never send messages")
	if flags.Parse(args) != nil {
		return 2
	}
	if *id == "" {
		fmt.Fprintln(out, "configuration: --bot is required; no network used")
		return 2
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(out, "configuration: local file unavailable; no network used")
		return 3
	}
	var cfg config.Config
	if json.Unmarshal(raw, &cfg) != nil {
		fmt.Fprintln(out, "configuration: invalid JSON; no network used")
		return 3
	}
	for _, bot := range cfg.Bots {
		if bot.ID == *id {
			b, err := core.NewBot(bot)
			if err != nil {
				fmt.Fprintln(out, "configuration: credentials or mode missing/invalid; live verification not executed")
				return 4
			}
			fmt.Fprintf(out, "configuration: valid; mode=%s; runtime/callback/client delivery: unverified\n", b.Mode)
			if !*live {
				fmt.Fprintln(out, "offline: no QQ requests made; real integration not executed")
				return 0
			}
			if _, err = b.TokenSource.Token(); err != nil {
				fmt.Fprintln(out, "live authentication failed; inspect credentials, network and platform policy locally; no messages sent")
				return 5
			}
			fmt.Fprintln(out, "live authentication: passed; callback and QQ client delivery remain unverified; no messages sent")
			return 0
		}
	}
	fmt.Fprintln(out, "configuration: selected Bot not found; no network used")
	return 4
}
func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
