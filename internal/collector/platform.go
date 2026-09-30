package collector

type platformCollector interface {
	Collect() (HostMetrics, ContainerMetrics, StorageMetrics)
}
