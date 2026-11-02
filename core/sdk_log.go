package core

import (
	sdklog "github.com/kylin930/botgo-plus/log"
	"log"
)

// Preserve the logger hook as defense in depth across SDK revisions. Per-Bot
// diagnostics are reported separately; arbitrary SDK arguments are not forwarded.
func init() { sdklog.DefaultLogger = safeSDKLogger{} }

type safeSDKLogger struct{}

func (safeSDKLogger) Debug(...interface{})          {}
func (safeSDKLogger) Debugf(string, ...interface{}) {}
func (safeSDKLogger) Info(...interface{})           {}
func (safeSDKLogger) Infof(string, ...interface{})  {}
func (safeSDKLogger) Warn(...interface{})           { log.Print("QQ SDK warning") }
func (safeSDKLogger) Warnf(string, ...interface{})  { log.Print("QQ SDK warning") }
func (safeSDKLogger) Error(...interface{})          { log.Print("QQ SDK operation failed") }
func (safeSDKLogger) Errorf(string, ...interface{}) { log.Print("QQ SDK operation failed") }
func (safeSDKLogger) Sync() error                   { return nil }
