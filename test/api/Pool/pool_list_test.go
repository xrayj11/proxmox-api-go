package api_test

import (
	"testing"

	"github.com/xrayj11/proxmox-api-go/proxmox"
	api_test "github.com/xrayj11/proxmox-api-go/test/api"
	"github.com/stretchr/testify/require"
)

func Test_Pools_List(t *testing.T) {
	Test := api_test.Test{}
	_ = Test.CreateTest()
	pools, err := proxmox.ListPools(Test.GetClient())
	require.NoError(t, err)
	require.Equal(t, 1, len(pools))
}
