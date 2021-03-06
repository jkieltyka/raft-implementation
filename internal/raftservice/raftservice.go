package raftservice

import (
	"os"
	"time"

	"github.com/jkieltyka/raft-implementation/internal/raftclient"
	"github.com/jkieltyka/raft-implementation/internal/raftserver"
	"github.com/jkieltyka/raft-implementation/pkg/backoff"
	"github.com/jkieltyka/raft-implementation/pkg/discovery"
)

func StartService() error {
	time.Sleep(5 * time.Second)
	//Start RaftServer
	rs := raftserver.NewServer()
	go rs.Start()

	//Get Raft Nodes
	k8sdiscover := discovery.Kubernetes{}
	// nodes := k8sdiscover.GetNodes("raft", "default")
	nodes := k8sdiscover.GetNodes("raft", "default")
	var raftClients raftclient.ClientList = nodes

	//Start RandomBackoff
	backoffSettings := backoff.NewBackoff(2000, 7000, os.Getenv("POD_NAME"))
	backoffTimer := backoffSettings.SetBackoff()

	heartbeatTimer := time.NewTimer(500 * time.Millisecond)
	heartbeatTimer.Stop()

	for {
		select {
		case <-heartbeatTimer.C:
			if rs.State.Role == "leader" {
				raftClients.SendHeartbeat(rs.State)
				rs.Heartbeat <- true
				heartbeatTimer = time.NewTimer(500 * time.Millisecond)
			}
		case <-backoffTimer.C:
			//set new election backoff
			backoffTimer = backoffSettings.SetBackoff()
			//Send candidate message to nodes
			if rs.State.Role == "follower" || rs.State.Role == "leader" {
				rs.State.Role = "candidate"
				rs.RoleChange <- "candidate"
				rs.State.CurrentTerm++
			}
			accepted := raftClients.RequestVote(rs.State)
			if accepted {
				rs.State.Role = "leader"
				rs.RoleChange <- "leader"
				rs.State.CurrentLeaderID = os.Getenv("POD_NAME")
				raftClients.SendHeartbeat(rs.State)
				rs.Heartbeat <- true
				heartbeatTimer = time.NewTimer(500 * time.Millisecond)
			}
			rs.State.VotedForID = nil

		case <-rs.Heartbeat:
			backoffSettings.ResetBackoff(backoffTimer)
		}
	}
}
