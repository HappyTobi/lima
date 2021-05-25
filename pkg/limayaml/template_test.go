package limayaml

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestDefaultTemplateQemuYAML(t *testing.T) {
	_, err := Load(defaultTemplateQemu)
	assert.NilError(t, err)
	// Do not call Validate(y) here, as it fails when `~/lima` is missing
}

func TestDefaultTemplateMacvirtYAML(t *testing.T) {
	_, err := Load(defaultTemplateMacVirt)
	assert.NilError(t, err)
	// Do not call Validate(y) here, as it fails when `~/lima` is missing
}
