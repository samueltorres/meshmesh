package gossip

import (
	"encoding/json"
)

type NodeMeta struct {
	ClusterName string
}

func (m NodeMeta) Bytes() []byte {
	data, err := json.Marshal(m)
	if err != nil {
		return []byte("")
	}
	return data
}

func ParseNodeMeta(data []byte) (NodeMeta, error) {
	meta := NodeMeta{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

type ClusterMeshAPIServerData struct {
	ClusterName string
}
