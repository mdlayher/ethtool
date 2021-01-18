//+build !linux

package ethtool

import (
	"fmt"
	"runtime"
)

// errUnsupported indicates that this library is not functional on non-Linux
// platforms.
var errUnsupported = fmt.Errorf("ethtool: this library is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)

type client struct{}

func newClient() (*client, error)                           { return nil, errUnsupported }
func (c *client) LinkInfos() ([]*LinkInfo, error)           { return nil, errUnsupported }
func (c *client) LinkInfo(_ Interface) (*LinkInfo, error)   { return nil, errUnsupported }
func (c *client) LinkModes() ([]*LinkMode, error)           { return nil, errUnsupported }
func (c *client) LinkMode(_ Interface) (*LinkMode, error)   { return nil, errUnsupported }
func (c *client) WakeOnLANs() ([]*WakeOnLAN, error)         { return nil, errUnsupported }
func (c *client) WakeOnLAN(_ Interface) (*WakeOnLAN, error) { return nil, errUnsupported }
func (c *client) SetWakeOnLAN(_ WakeOnLAN) error            { return errUnsupported }
func (c *client) Close() error                              { return errUnsupported }
