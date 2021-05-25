package start

import (
	"github.com/AkihiroSuda/lima/pkg/limayaml"
)

type Emulator interface {
	Start(instName, instDir string, y *limayaml.LimaYAML) error
	EmulatorName() string
}
