package common

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

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

const linodeBSLabelLength = 32

func hashStringToInt(b string) int {
	algorithm := fnv.New32a()
	_, _ = algorithm.Write([]byte(b))
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

	volID := 0
	if key, err := ParseLinodeVolumeKey(strVolID); err == nil {
		volID = key.GetVolumeID()
	} else {
		// hack to permit csi-test to use ill-formatted volumeids
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

type LinodeVolumeKey struct {
	VolumeID int
	Label    string
}

func CreateLinodeVolumeKey(id int, label string) LinodeVolumeKey {
	return LinodeVolumeKey{id, label}
}

func ParseLinodeVolumeKey(key string) (*LinodeVolumeKey, error) {
	keys := strings.SplitN(key, "-", 2)
	if len(keys) != 2 {
		return nil, fmt.Errorf("Invalid Linode Volume key: %q", key)
	}

	volumeID, err := strconv.Atoi(keys[0])
	if err != nil {
		return nil, fmt.Errorf("Invalid Linode Volume ID: %q", keys[0])
	}

	lvk := LinodeVolumeKey{volumeID, keys[1]}
	return &lvk, nil
}

func (key *LinodeVolumeKey) GetVolumeID() int {
	return key.VolumeID
}

func (key *LinodeVolumeKey) GetVolumeLabel() string {
	return key.Label
}

func (key *LinodeVolumeKey) GetNormalizedLabel() string {
	volumeName := strings.Replace(key.Label, "-", "", -1)
	if len(volumeName) > linodeBSLabelLength {
		volumeName = volumeName[:linodeBSLabelLength]
	}

	return volumeName
}

func (key *LinodeVolumeKey) GetNormalizedLabelWithPrefix(prefix string) string {
	label := prefix + key.GetNormalizedLabel()
	if len(label) > linodeBSLabelLength {
		label = label[:linodeBSLabelLength]
	}
	return label
}

func (key *LinodeVolumeKey) GetVolumeKey() string {
	volumeName := key.GetNormalizedLabel()
	return fmt.Sprintf("%d-%s", key.VolumeID, volumeName)
}
