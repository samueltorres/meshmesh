package state

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

func NewClusterMeshState(logger *zap.Logger) *ClusterMeshState {
	return &ClusterMeshState{
		mux:      &sync.RWMutex{},
		clusters: make(map[string]*ClusterMeshCluster),
		updateCh: make(chan struct{}),
		logger:   logger,
	}
}

type ClusterMeshState struct {
	mux      *sync.RWMutex
	clusters map[string]*ClusterMeshCluster
	updateCh chan struct{}
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
	p.mux.Lock()
	defer p.mux.Unlock()
	p.clusters[cluster.ClusterName] = cluster

	p.updateCh <- struct{}{}
	return nil
}

func (p *ClusterMeshState) Delete(clusterName string) {
	p.mux.Lock()
	defer p.mux.Unlock()

	if c, ok := p.clusters[clusterName]; ok {
		c.Tombstone = time.Now().UnixNano()
	}

	p.updateCh <- struct{}{}
}

func (p *ClusterMeshState) GetAll() map[string]*ClusterMeshCluster {
	p.mux.RLock()
	defer p.mux.RUnlock()

	meshClusters := make(map[string]*ClusterMeshCluster, len(p.clusters))
	for k, c := range p.clusters {
		meshClusters[k] = c
	}

	return meshClusters
}

func (p *ClusterMeshState) UpdateChannel() chan struct{} {
	return p.updateCh
}
