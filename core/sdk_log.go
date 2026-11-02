package core

import (
	sdklog "github.com/kylin930/botgo-plus/log"
	"log"
)

// The external SDK logs credential request bodies and access tokens at Debug.
// Its logger hook is configured once before any bots start. Business errors are
// reported separately with BotID; SDK diagnostics must never include raw arguments.
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
