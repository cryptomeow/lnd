package lncfg

import "github.com/cryptomeow/lnd/watchtower"

// Watchtower holds the daemon specific configuration parameters for running a
// watchtower that shares resources with the daemon.
type Watchtower struct {
	Active bool `long:"active" description:"If the watchtower should be active or not"`

	TowerDir string `long:"towerdir" description:"Directory of the watchtower.db"`

	watchtower.Conf
}
