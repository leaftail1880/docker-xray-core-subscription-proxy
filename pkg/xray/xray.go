package xray

import (
	"github.com/xtls/xray-core/core"
	"xray-balancer/pkg/logger"
)

// Start creates and starts an Xray instance with the given config.
func Start(config *core.Config) (*core.Instance, error) {
	server, err := core.New(config)
	if err != nil {
		return nil, err
	}
	if err := server.Start(); err != nil {
		return nil, err
	}
	return server, nil
}

// Restart stops the old server and starts a new one with the new config.
// It returns the new server or an error if the new server couldn't start.
func Restart(old *core.Instance, newConfig *core.Config) (*core.Instance, error) {
	logger.Info.Println("Restarting Xray core...")
	if old != nil {
		old.Close()
	}
	return Start(newConfig)
}
