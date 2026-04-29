package compose

import "strings"

// statefulImagePrefixes lists image name prefixes (after registry/owner stripping)
// for services that must not run multiple replicas. Matched as a prefix on the
// final image name before any tag, so "library/postgres", "postgres:16",
// "bitnami/postgresql" all match.
var statefulImagePrefixes = []string{
	"postgres", "postgresql",
	"mysql", "mariadb", "percona",
	"mongo",
	"redis", "valkey", "dragonfly",
	"elasticsearch", "opensearch",
	"clickhouse", "cassandra", "scylla",
	"rabbitmq", "kafka", "nats", "pulsar",
	"etcd", "consul", "zookeeper",
	"neo4j", "arangodb", "surrealdb",
	"influxdb", "victoriametrics", "timescale", "questdb", "prometheus",
	"qdrant", "weaviate", "milvus",
	"minio", "seaweedfs",
	"cockroach", "tidb", "yugabyte",
}

// ScaleEligibility reports whether the service can safely run with N>1
// replicas. Returns (true, "") when scalable, (false, reason) otherwise.
//
// Detection rules (in order):
//  1. simpledeploy.scalable=false label forces non-scalable.
//  2. simpledeploy.scalable=true label forces scalable (override).
//  3. deploy.mode: global (one container per node).
//  4. Any host-published port (would clash on the same host).
//  5. Named volumes (typically stateful).
//  6. Known stateful image (database, broker, etc.).
func (s *ServiceConfig) ScaleEligibility() (bool, string) {
	if s == nil {
		return false, "unknown service"
	}
	if v, ok := s.Labels["simpledeploy.scalable"]; ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "false", "0", "no":
			return false, "marked non-scalable by app config"
		case "true", "1", "yes":
			return true, ""
		}
	}
	if strings.EqualFold(s.DeployMode, "global") {
		return false, "deploy mode is global"
	}
	for _, p := range s.Ports {
		if strings.TrimSpace(p.Host) != "" {
			return false, "publishes host port " + p.Host
		}
	}
	for _, v := range s.Volumes {
		if v.Type == "volume" && strings.TrimSpace(v.Source) != "" {
			return false, "uses persistent volume"
		}
	}
	if reason := matchStatefulImage(s.Image); reason != "" {
		return false, reason
	}
	return true, ""
}

func matchStatefulImage(image string) string {
	if image == "" {
		return ""
	}
	name := image
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.IndexAny(name, ":@"); i >= 0 {
		name = name[:i]
	}
	name = strings.ToLower(name)
	// Exact match only. Substring/prefix matching would catch sidecars like
	// postgres-exporter or prometheus-node-exporter, which are stateless.
	for _, p := range statefulImagePrefixes {
		if name == p {
			return "stateful image (" + p + ")"
		}
	}
	return ""
}
