package limayaml

import (
	_ "embed"
)

//go:embed default.TEMPLATE.qemu.yaml
var defaultTemplateQemu []byte

//go:embed default.TEMPLATE.macvirt.yaml
var defaultTemplateMacVirt []byte

func DefaultTemplateForEmulator(emuName string) []byte {
	if emuName == "macvirt" {
		return defaultTemplateMacVirt
	}
	return defaultTemplateQemu
}
