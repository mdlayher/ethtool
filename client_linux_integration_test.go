//go:build linux
// +build linux

package ethtool_test

import (
	"testing"

	"github.com/mdlayher/ethtool"
)

func TestIntegrationClientLinkInfos(t *testing.T) {
	// Make sure the basic netlink plumbing is in place without requiring the
	// use of any privileged operations.
	c, err := ethtool.New()
	if err != nil {
		t.Fatalf("failed to open client: %v", err)
	}
	defer c.Close()

	lis, err := c.LinkInfos()
	if err != nil {
		t.Fatalf("failed to fetch link infos: %v", err)
	}

	for _, li := range lis {
		t.Logf("%d: %q: %s", li.Interface.Index, li.Interface.Name, li.Port)
	}
}
