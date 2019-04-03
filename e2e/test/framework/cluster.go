package framework

func CreateCluster(cluster string) error {
	return RunScript("create_cluster.sh", ApiToken, cluster)
}

func DeleteCluster(cluster string) error {
	return RunScript("delete_cluster.sh", cluster)
}
