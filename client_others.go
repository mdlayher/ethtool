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

func newClient() (*client, error)                       { return nil, errUnsupported }
func (c *client) LinkInfos() ([]*LinkInfo, error)       { return nil, errUnsupported }
func (c *client) LinkInfo(_ Request) (*LinkInfo, error) { return nil, errUnsupported }
func (c *client) Close() error                          { return errUnsupported }
