package gossip

import (
	"encoding/json"

	"github.com/hashicorp/memberlist"
	"github.com/samueltorres/meshmesh/internal/state"
	"go.uber.org/zap"
)

func NewDelegate(logger *zap.Logger, meshState *state.ClusterMeshState) *Delegate {
	return &Delegate{
		logger:    logger,
		nodeMeta:  NodeMeta{},
		meshState: meshState,
	}
}

type Delegate struct {
	logger    *zap.Logger
	nodeMeta  NodeMeta
	meshState *state.ClusterMeshState
}

func (e *Delegate) NotifyJoin(node *memberlist.Node) {
	e.logger.Info("node joined: " + node.FullAddress().Name)
}

func (e *Delegate) NotifyLeave(node *memberlist.Node) {
	e.logger.Info("node leave: " + node.FullAddress().Name)
}

func (e *Delegate) NotifyUpdate(node *memberlist.Node) {
	e.logger.Info("node update: " + node.FullAddress().Name)
}

func (d *Delegate) NodeMeta(limit int) []byte {
	return d.nodeMeta.Bytes()
}

func (d *Delegate) LocalState(join bool) []byte {
	d.logger.Debug("LocalState", zap.Bool("join", join))
	b, err := json.Marshal(d.meshState.GetAll())
	if err != nil {
		d.logger.Error("error mashalling local state", zap.Error(err))
	}
	return b
}

func (d *Delegate) MergeRemoteState(buf []byte, join bool) {
	d.logger.Debug("MergeRemoteState", zap.Bool("join", join))

	if len(buf) == 0 {
		d.logger.Info("empty buf", zap.Bool("join", join))
	}

	clusters := map[string]*state.ClusterMeshCluster{}
	err := json.Unmarshal(buf, &clusters)
	if err != nil {
		d.logger.Error("unable to unmarshall remote state", zap.Error(err))
		return
	}

	for _, c := range clusters {
		d.meshState.AddOrUpdate(c)
	}
}

func (d *Delegate) NotifyMsg(msg []byte) {
}

func (d *Delegate) GetBroadcasts(overhead, limit int) [][]byte {
	return nil
}
