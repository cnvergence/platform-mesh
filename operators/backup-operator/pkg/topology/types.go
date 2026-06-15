package topology

import (
	"time"
)

// Manifest is the in-memory representation of topology.json.
type Manifest struct {
	SchemaVersion   string          `json:"schemaVersion"`
	CapturedAt      time.Time       `json:"capturedAt"`
	HostCluster     HostCluster     `json:"hostCluster"`
	Kcp             KcpTopology     `json:"kcp"`
	CNPG            CNPGTopology    `json:"cnpg"`
	OpenFGA         OpenFGATopology `json:"openfga"`
	OperatorVersion string          `json:"operatorVersion"`
}

type HostCluster struct {
	KubernetesVersion string `json:"kubernetesVersion"`
	Namespace         string `json:"namespace"`
}

type KcpTopology struct {
	ShardCount int        `json:"shardCount"`
	Shards     []KcpShard `json:"shards"`
}

type KcpShard struct {
	Name                    string `json:"name"`
	EtcdRef                 string `json:"etcdRef"`
	LogicalClusterIDsDigest string `json:"logicalClusterIDsDigest"`
}

type CNPGTopology struct {
	Clusters []CNPGCluster `json:"clusters"`
}

type CNPGCluster struct {
	Name         string `json:"name"`
	SpecDigest   string `json:"specDigest"`
	MajorVersion int    `json:"majorVersion"`
}

type OpenFGATopology struct {
	Stores []OpenFGAStore `json:"stores"`
}

type OpenFGAStore struct {
	Name        string `json:"name"`
	ModelDigest string `json:"modelDigest"`
}
