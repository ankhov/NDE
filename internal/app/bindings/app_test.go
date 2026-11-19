package bindings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppServiceVersion(t *testing.T) {
	s := NewAppService("1.2.3")
	assert.Equal(t, "1.2.3", s.Version())
}

func TestAppServiceHealth(t *testing.T) {
	s := NewAppService("1.2.3")
	h := s.Health()
	assert.True(t, h.Ok)
	assert.Equal(t, "1.2.3", h.Version)
	assert.NotEmpty(t, h.StartedAt)
}
