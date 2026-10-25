package fakeprovider_test

import (
	"testing"

	"github.com/Paca-AI/plugin-sdk-go/gitprovider/conformance"
	"github.com/Paca-AI/plugin-sdk-go/gitprovider/fakeprovider"
)

// The full fake (every optional capability) and the minimal fake (none) must
// both pass: the suite has to work for rich and basic platforms alike.
func TestConformance_Full(t *testing.T)    { conformance.Run(t, fakeprovider.Harness(false)) }
func TestConformance_Minimal(t *testing.T) { conformance.Run(t, fakeprovider.Harness(true)) }
