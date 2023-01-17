package framework

func CreateCluster(cluster string, region, k8s_version string) error {
	return RunScript("create_cluster.sh", ApiToken, cluster, k8s_version, region)
}

func DeleteCluster(cluster string) error {
	return RunScript("delete_cluster.sh", cluster)
}
