package gossip

import (
	"encoding/json"

	"github.com/hashicorp/memberlist"
	"github.com/samueltorres/meshmesh/internal/state"
	"go.uber.org/zap"
)

func NewEventDelegate(logger *zap.Logger, localClusterName string, meshState *state.ClusterMeshState) *EventDelegate {
	return &EventDelegate{
		logger:           logger,
		localClusterName: localClusterName,
		meshState:        meshState,
	}
}

type EventDelegate struct {
	logger           *zap.Logger
	localClusterName string
	meshState        *state.ClusterMeshState
}

func (e *EventDelegate) NotifyJoin(node *memberlist.Node) {
	e.logger.Info("node joined: " + node.FullAddress().Name)
}

func (e *EventDelegate) NotifyLeave(node *memberlist.Node) {
	e.logger.Info("node leave: " + node.FullAddress().Name)
}

func (e *EventDelegate) NotifyUpdate(node *memberlist.Node) {
	e.logger.Info("node update: " + node.FullAddress().Name)
}

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

func (d *Delegate) NodeMeta(limit int) []byte {
	return d.nodeMeta.Bytes()
}

func (d *Delegate) LocalState(join bool) []byte {
	d.logger.Info("localstate")
	d.meshState.RLock()
	defer d.meshState.RUnlock()
	b, err := json.Marshal(d.meshState.GetAll())

	if err != nil {
		d.logger.Error("marshalling")
	}

	d.meshState.PrintMembers()
	return b
}

func (d *Delegate) MergeRemoteState(buf []byte, join bool) {
	d.logger.Info("MergeRemoteState", zap.Bool("join", join))

	if len(buf) == 0 {
		d.logger.Info("empty buf", zap.Bool("join", join))
	}

	d.meshState.Lock()
	defer d.meshState.Unlock()
	clusters := map[string]*state.ClusterMeshCluster{}
	err := json.Unmarshal(buf, &clusters)
	if err != nil {
		d.logger.Error("unable to unmarshall remote state", zap.Error(err))
		return
	}

	for _, c := range clusters {
		d.meshState.AddOrUpdate(c)
	}

	d.meshState.PrintMembers()
}

func (d *Delegate) NotifyMsg(msg []byte) {
}

func (d *Delegate) GetBroadcasts(overhead, limit int) [][]byte {
	// not use, noop
	return nil
}
