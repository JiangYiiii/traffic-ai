package health

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMountProbesEmptyPrefixNoDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("")
	MountProbes(r, g, "", nil, nil)
}

func TestMountProbesWithPrefixRegistersRootAndGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/traffic-console")
	MountProbes(r, g, "/traffic-console", nil, nil)
}
