package omap_test

import (
	"testing"

	"github.com/gasmod/omap"
)

func TestMap(t *testing.T) {
	runConformance(t, func() orderedMap[string, int] { return &omap.Map[string, int]{} })
}

func TestMapFloatKeys(t *testing.T) {
	runFloatKeyConformance(t, func() orderedMap[float64, int] { return &omap.Map[float64, int]{} })
}

func TestMapClone(t *testing.T) {
	testClone(t,
		func() *omap.Map[string, int] { return &omap.Map[string, int]{} },
		(*omap.Map[string, int]).Clone,
	)
}
