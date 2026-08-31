package hwinfo

import (
	"github.com/jaypipes/ghw"
)

type HardwareInfo interface {
	Block() (*ghw.BlockInfo, error)
}

func Block() (*ghw.BlockInfo, error) {
	return ghw.Block()
}

type hwInfo struct{}

func (h *hwInfo) Block() (*ghw.BlockInfo, error) {
	return ghw.Block()
}

func NewHardwareInfo() HardwareInfo {
	return &hwInfo{}
}
