package common

import (
	"hash/fnv"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type (
	withVolume interface {
		GetVolumeId() string
	}

	withNode interface {
		GetNodeId() string
	}
)

func hashStringToInt(b string) int {
	algorithm := fnv.New32a()
	algorithm.Write([]byte(b))
	i := algorithm.Sum32()
	return int(i)
}

func VolumeIdAsInt(caller string, w withVolume) (int, error) {
	strVolID := w.GetVolumeId()
	if len(caller) != 0 {
		caller = caller + " "
	}

	if len(strVolID) == 0 {
		return 0, status.Errorf(codes.InvalidArgument, "%sVolume ID must be provided", caller)
	}

	volID, err := strconv.Atoi(strVolID)
	if err != nil {
		volID = hashStringToInt(strVolID)
	}

	return volID, nil
}

func NodeIdAsInt(caller string, w withNode) (int, error) {
	strNodeID := w.GetNodeId()
	if len(caller) != 0 {
		caller = caller + " "
	}

	if len(strNodeID) == 0 {
		return 0, status.Errorf(codes.InvalidArgument, "%sNode ID must be provided", caller)
	}

	nodeID, err := strconv.Atoi(strNodeID)
	if err != nil {
		nodeID = hashStringToInt(strNodeID)
	}

	return nodeID, nil
}
