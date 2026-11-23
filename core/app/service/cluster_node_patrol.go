package service

import (
	"sync"
	"time"

	"github.com/1Panel-dev/1Panel/core/global"
)

// Cluster node status patrol.
//
// Node status is persisted by probeNode and then trusted by the monitor and
// the alert sampler: once a node is stored as offline they stop calling it,
// so it can never come back on its own. This patrol re-probes every
// registered node on a fixed interval and writes the result back, which is
// what makes an offline node recover without anyone opening the page.
const clusterNodePatrolTick = 60 * time.Second

var clusterNodePatrolOnce sync.Once

// StartClusterNodePatrol starts the background status patrol once.
func StartClusterNodePatrol() {
	clusterNodePatrolOnce.Do(func() {
		go clusterNodePatrolLoop()
	})
}

func clusterNodePatrolLoop() {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("cluster node patrol panic: %v", r)
		}
	}()
	// Let the database and the remote panels come up first.
	time.Sleep(25 * time.Second)
	ticker := time.NewTicker(clusterNodePatrolTick)
	defer ticker.Stop()
	for {
		clusterNodePatrolRun()
		<-ticker.C
	}
}

func clusterNodePatrolRun() {
	defer func() {
		if r := recover(); r != nil {
			global.LOG.Errorf("cluster node patrol run panic: %v", r)
		}
	}()
	if err := NewIClusterNodeService().CheckAll(); err != nil {
		global.LOG.Debugf("cluster node patrol: check failed: %v", err)
	}
}
