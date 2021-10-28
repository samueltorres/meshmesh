package state

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

func NewClusterMeshState(logger *zap.Logger) *ClusterMeshState {
	return &ClusterMeshState{
		mux:      &sync.RWMutex{},
		clusters: make(map[string]*ClusterMeshCluster),
		logger:   logger,
	}
}

type ClusterMeshState struct {
	mux      *sync.RWMutex
	clusters map[string]*ClusterMeshCluster
	logger   *zap.Logger
}

type ClusterMeshCluster struct {
	ClusterName                 string `json:",omitempty"`
	ClusterMeshAPIServerIP      string `json:",omitempty"`
	ClusterMeshAPIServerAddress string `json:",omitempty"`
	ClusterMeshAPIServerTLSCA   string `json:",omitempty"`
	ClusterMeshAPIServerTLSCert string `json:",omitempty"`
	ClusterMeshAPIServerTLSKey  string `json:",omitempty"`
	Tombstone                   int64  `json:",omitempty"`
}

func (p *ClusterMeshState) AddOrUpdate(cluster *ClusterMeshCluster) error {
	p.clusters[cluster.ClusterName] = cluster

	return nil
}

func (p *ClusterMeshState) Delete(clusterName string) {
	if c, ok := p.clusters[clusterName]; ok {
		c.Tombstone = time.Now().UnixNano()
	}
}

func (p *ClusterMeshState) GetAll() map[string]*ClusterMeshCluster {
	return p.clusters
}

func (p *ClusterMeshState) PrintMembers() {
	fmt.Println(p.clusters)
}

func (p *ClusterMeshState) RLock() {
	p.mux.RLock()
}

func (p *ClusterMeshState) RUnlock() {
	p.mux.RUnlock()
}

func (p *ClusterMeshState) Lock() {
	p.mux.Lock()
}

func (p *ClusterMeshState) Unlock() {
	p.mux.Unlock()
}
