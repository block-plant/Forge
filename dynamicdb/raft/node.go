package raft

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/dynamicdb/lsm"
)

// State traces node roles within the consensus cluster.
type State int

const (
	Follower State = iota
	Candidate
	Leader
)

// Node encompasses physical machine boundaries operating Raft consensus.
type Node struct {
	mu          sync.RWMutex
	state       State
	currentTerm int
	votedFor    string
	commits      []string // Replicated logs

	wal         *lsm.WAL
	id          string
	peers       []string
}

// Start boots the cluster consensus protocols. Wait for heartbeat timeouts.
func (n *Node) Start() {
	go n.runElectionTimer()
}

func (n *Node) runElectionTimer() {
	timeout := time.Duration(150+rand.Intn(150)) * time.Millisecond
	time.Sleep(timeout)

	n.mu.Lock()
	if n.state != Leader {
		n.state = Candidate
		n.currentTerm++
		n.votedFor = n.id
		// Emulate broadcasting RequestVote RPCs securely ensuring distributed
		fmt.Printf("DynamicDB Raft: Node %s initializing election for Term %d\n", n.id, n.currentTerm)
	}
	n.mu.Unlock()
}
